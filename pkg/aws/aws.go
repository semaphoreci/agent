package aws

import (
	"fmt"
	"os/exec"
	"strings"

	versions "github.com/hashicorp/go-version"
	log "github.com/sirupsen/logrus"
)

func GetECRLoginCmd() (string, error) {
	version, _ := versions.NewVersion("1.17.10")
	awsCLIVersion, err := findAWSCLIVersion()
	if err != nil {
		return "", err
	}

	if awsCLIVersion.GreaterThanOrEqual(version) {
		/*
		 * get-login-password was added in v1.17.10 and is the only command available in v2.
		 * That command doesn't generate a docker login command by itself, only the password.
		 * So we need to pipe that into the docker login command ourselves.
		 * See: https://docs.aws.amazon.com/cli/latest/reference/ecr/get-login-password.html.
		 * The only difference here is that we need to determine the AWS account id for ourselves.
		 */
		accountId, err := findAWSAccountId()
		if err != nil {
			return "", err
		}

		return fmt.Sprintf(
			`aws ecr get-login-password --region $AWS_REGION | docker login --username AWS --password-stdin %s.dkr.ecr.$AWS_REGION.amazonaws.com`,
			accountId,
		), nil
	}

	/*
	 * get-login is only available in AWS CLI v1.
	 * The way it works is it generates a token, and then prints the
	 * docker login command to actually login. Note the extra $() around it.
	 * This is to make sure we execute the output of that command as well.
	 * See: https://docs.aws.amazon.com/cli/latest/reference/ecr/get-login.html
	 */
	return `$(aws ecr get-login --no-include-email --region $AWS_REGION)`, nil
}

func findAWSAccountId() (string, error) {
	cmd := exec.Command("bash", "-c", "aws sts get-caller-identity --query Account --output text")
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Errorf("Error finding AWS account ID: %v", err)
		return "", err
	}

	return strings.TrimSuffix(string(output), "\n"), nil
}

func findAWSCLIVersion() (*versions.Version, error) {
	cmd := exec.Command("bash", "-c", `aws --version 2>&1 | awk -F'[/ ]' '{print $2}'`)
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Errorf("Error determing AWS CLI version: %v", err)
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
