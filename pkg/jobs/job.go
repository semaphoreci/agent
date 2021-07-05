package jobs

import (
	"bytes"
	"fmt"
	"log"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	executors "github.com/semaphoreci/agent/pkg/executors"
	pester "github.com/sethgrid/pester"
)

const JOB_PASSED = "passed"
const JOB_FAILED = "failed"

type Job struct {
	Request *api.JobRequest
	Logger  *eventlogger.Logger

	Executor executors.Executor

	JobLogArchived bool
	Stopped        bool
	Finished       bool

	//
	// The Teardown phase can be entered either after:
	//  - a regular job execution ends
	//  - from the Job.Stop procedure
	//
	// With this lock, we are making sure that only one Teardown is
	// executed. This solves the race condition where both the job finishes
	// and the job stops at the same time.
	//
	TeardownLock Lock
}

func NewJob(request *api.JobRequest) (*Job, error) {
	log.Printf("Constructing an executor for the job")

	if request.Executor == "" {
		request.Executor = executors.ExecutorTypeShell
	}

	if request.Logger.Type == "" {
		request.Logger.Type = eventlogger.LoggerTypeFile
	}

	logger, err := eventlogger.CreateLogger(request)
	if err != nil {
		return nil, err
	}

	executor, err := executors.CreateExecutor(request, logger)
	if err != nil {
		return nil, err
	}

	log.Printf("Job Request %+v\n", request)
	log.Printf("Constructed job")

	return &Job{
		Request:        request,
		Executor:       executor,
		JobLogArchived: false,
		Stopped:        false,
		Logger:         logger,
	}, nil
}

func (job *Job) Run() {
	log.Printf("Job Started")
	executorRunning := false
	result := JOB_FAILED

	job.Logger.LogJobStarted()

	exitCode := job.PrepareEnvironment()
	if exitCode == 0 {
		executorRunning = true
	} else {
		log.Printf("Executor failed to boot up")
	}

	if executorRunning {
		result = job.RunRegularCommands()

		log.Printf("Regular Commands Finished. Result: %s", result)

		log.Printf("Exporting job result")

		job.RunCommandsUntilFirstFailure([]api.Command{
			{
				Directive: fmt.Sprintf("export SEMAPHORE_JOB_RESULT=%s", result),
			},
		})

		log.Printf("Starting Epilogue Always Commands.")
		job.RunCommandsUntilFirstFailure(job.Request.EpilogueAlwaysCommands)

		if result == JOB_PASSED {
			log.Printf("Starting Epilogue On Pass Commands.")
			job.RunCommandsUntilFirstFailure(job.Request.EpilogueOnPassCommands)
		} else {
			log.Printf("Starting Epilogue On Fail Commands.")
			job.RunCommandsUntilFirstFailure(job.Request.EpilogueOnFailCommands)
		}
	}

	job.Finished = true

	job.Teardown(result)
}

func (job *Job) PrepareEnvironment() int {
	exitCode := job.Executor.Prepare()
	if exitCode != 0 {
		log.Printf("Failed to prepare executor")
		return exitCode
	}

	exitCode = job.Executor.Start()
	if exitCode != 0 {
		log.Printf("Failed to start executor")
		return exitCode
	}

	return 0
}

func (job *Job) RunRegularCommands() string {
	exitCode := job.Executor.ExportEnvVars(job.Request.EnvVars)
	if exitCode != 0 {
		log.Printf("Failed to export env vars")

		return JOB_FAILED
	}

	exitCode = job.Executor.InjectFiles(job.Request.Files)
	if exitCode != 0 {
		log.Printf("Failed to inject files")

		return JOB_FAILED
	}

	exitCode = job.RunCommandsUntilFirstFailure(job.Request.Commands)

	if exitCode == 0 {
		return JOB_PASSED
	} else {
		return JOB_FAILED
	}
}

// returns exit code of last executed command
func (job *Job) RunCommandsUntilFirstFailure(commands []api.Command) int {
	lastExitCode := 1

	for _, c := range commands {
		if job.Stopped {
			return 1
		}

		lastExitCode = job.Executor.RunCommand(c.Directive, false, c.Alias)

		if lastExitCode != 0 {
			break
		}
	}

	return lastExitCode
}

func (job *Job) Teardown(result string) {
	if !job.TeardownLock.TryLock() {
		log.Printf("[warning] Duplicate attempts to enter the Teardown phase")
		return
	}

	log.Printf("Job Teardown Started")

	log.Printf("Sending finished callback.")
	job.SendFinishedCallback(result)
	job.Logger.LogJobFinished(result)

	log.Printf("Waiting for archivator")

	for {
		if job.JobLogArchived {
			break
		} else {
			time.Sleep(1000 * time.Millisecond)
		}
	}

	job.SendTeardownFinishedCallback()

	log.Printf("Archivator finished")

	err := job.Logger.Close()
	if err != nil {
		log.Printf("Event Logger error %+v", err)
	}

	log.Printf("Job Teardown Finished")
}

func (j *Job) Stop() {
	log.Printf("Stopping job")

	j.Stopped = true

	log.Printf("Invoking process stopping")

	PreventPanicPropagation(func() {
		j.Executor.Stop()
	})

	log.Printf("Process stopping finished. Entering the Teardown phase.")

	j.Teardown("stopped")
}

func (job *Job) SendFinishedCallback(result string) error {
	payload := fmt.Sprintf(`{"result": "%s"}`, result)

	return job.SendCallback(job.Request.Callbacks.Finished, payload)
}

func (job *Job) SendTeardownFinishedCallback() error {
	return job.SendCallback(job.Request.Callbacks.TeardownFinished, "{}")
}

func (job *Job) SendCallback(url string, payload string) error {
	log.Printf("Sending callback: %s with %+v\n", url, payload)

	client := pester.New()
	client.MaxRetries = 100
	client.KeepLog = true

	resp, err := client.Post(url, "application/json", bytes.NewBuffer([]byte(payload)))

	log.Printf("%+v\n", resp)

	return err
}
