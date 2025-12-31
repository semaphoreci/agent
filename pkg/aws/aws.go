package aws

import (
	"fmt"
	"os/exec"
	"strings"

	versions "github.com/hashicorp/go-version"
	api "github.com/semaphoreci/agent/pkg/api"
	log "github.com/sirupsen/logrus"
)

func GetECRServerURL(credentials api.ImagePullCredentials) (string, error) {
	region, err := credentials.FindEnvVar("AWS_REGION")
	if err != nil {
		return "", err
	}

	accountID, err := GetAccountID(credentials)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", accountID, region), nil
}

func GetECRLoginPassword(credentials api.ImagePullCredentials) (string, error) {
	envs, err := credentials.ToCmdEnvVars()
	if err != nil {
		return "", err
	}

	cmd := exec.Command("bash", "-c", "aws ecr get-login-password --region $AWS_REGION")
	cmd.Env = envs
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error executing aws ecr get-login-password: Output: %s - Error: %v", string(output), err)
	}

	return strings.TrimSuffix(string(output), "\n"), nil
}

func GetECRLoginCmd(credentials api.ImagePullCredentials) (string, error) {
	awsV2, _ := versions.NewVersion("2.0.0")
	awsCLIVersion, err := findAWSCLIVersion()
	if err != nil {
		return "", err
	}

	if awsCLIVersion.GreaterThanOrEqual(awsV2) {
		accountID, err := GetAccountID(credentials)
		if err != nil {
			return "", err
		}

		/*
		 * get-login-password was added in v1.17.10 and is the only command available in v2.
		 * That command doesn't generate a docker login command by itself, only the password.
		 * So we need to pipe that into the docker login command ourselves.
		 * See: https://docs.aws.amazon.com/cli/latest/reference/ecr/get-login-password.html.
		 * The only difference here is that we need to determine the AWS account id for ourselves.
		 */
		return fmt.Sprintf(
			`aws ecr get-login-password --region $AWS_REGION | docker login --username AWS --password-stdin %s.dkr.ecr.$AWS_REGION.amazonaws.com`,
			accountID,
		), nil
	}

	/*
	 * get-login is only available in AWS CLI v1.
	 * The way it works is it generates a token, and then prints the
	 * docker login command to actually login. Note the extra $() around it.
	 * This is to make sure we execute the output of that command as well.
	 * See: https://docs.aws.amazon.com/cli/latest/reference/ecr/get-login.html
	 */
	accountID, err := credentials.FindEnvVar("AWS_ACCOUNT_ID")
	if err != nil {
		return `$(aws ecr get-login --no-include-email --region $AWS_REGION)`, nil
	}

	/*
	 * If AWS_ACCOUNT_ID is specified in the env vars, the registry is
	 * possibly living in a separate AWS account, so we set --registry-ids.
	 */
	return fmt.Sprintf(`$(aws ecr get-login --no-include-email --region $AWS_REGION --registry-ids %s)`, accountID), nil
}

func GetAccountID(credentials api.ImagePullCredentials) (string, error) {
	accountID, err := credentials.FindEnvVar("AWS_ACCOUNT_ID")
	if err == nil {
		return accountID, nil
	}

	accountID, err = getAccountIDFromSTS(credentials)
	if err != nil {
		return "", err
	}

	return accountID, nil
}

func getAccountIDFromSTS(credentials api.ImagePullCredentials) (string, error) {
	envs, err := credentials.ToCmdEnvVars()
	if err != nil {
		return "", err
	}

	cmd := exec.Command("bash", "-c", "aws sts get-caller-identity --query Account --output text")
	cmd.Env = envs

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Error finding AWS account ID: Output: %s - Error: %v", string(output), err)
		return "", err
	}

	return strings.TrimSuffix(string(output), "\n"), nil
}

func findAWSCLIVersion() (*versions.Version, error) {
	cmd := exec.Command("bash", "-c", `aws --version 2>&1 | awk -F'[/ ]' '{print $2}'`)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Error determing AWS CLI version: Output '%s' - Error: %v", string(output), err)
		return nil, err
	}

	versionAsString := strings.TrimSuffix(string(output), "\n")
	version, err := versions.NewVersion(versionAsString)
	if err != nil {
		log.Errorf("Error parsing AWS CLI version from '%s': %v", versionAsString, err)
		return nil, err
	}

	return version, nil
}
