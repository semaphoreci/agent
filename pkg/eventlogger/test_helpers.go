package eventlogger

import (
	"encoding/json"
	"fmt"
)

func TransformToObjects(events []string) ([]interface{}, error) {
	objects := []interface{}{}
	for _, event := range events {
		var object map[string]interface{}
		err := json.Unmarshal([]byte(event), &object)
		if err != nil {
			return []interface{}{}, err
		}

		switch eventType := object["event"].(string); {
		case eventType == "job_started":
			objects = append(objects, &JobStartedEvent{Event: eventType})
		case eventType == "job_finished":
			objects = append(objects, &JobFinishedEvent{Event: eventType, Result: object["result"].(string)})
		case eventType == "cmd_started":
			objects = append(objects, &CommandStartedEvent{Event: eventType, Directive: object["directive"].(string)})
		case eventType == "cmd_output":
			objects = append(objects, &CommandOutputEvent{Event: eventType, Output: object["output"].(string)})
		case eventType == "cmd_finished":
			objects = append(objects, &CommandFinishedEvent{Event: eventType, ExitCode: int(object["exit_code"].(float64))})
		}
	}

	return objects, nil
}

func SimplifyLogEvents(events []interface{}, includeOutput bool) ([]string, error) {
	simplified := []string{}

	output := ""

	for _, event := range events {
		switch e := event.(type) {
		case *JobStartedEvent:
			simplified = append(simplified, "job_started")
		case *JobFinishedEvent:
			simplified = append(simplified, "job_finished: "+e.Result)
		case *CommandStartedEvent:
			simplified = append(simplified, "directive: "+e.Directive)
		case *CommandOutputEvent:
			output = output + e.Output
		case *CommandFinishedEvent:
			if includeOutput {
				if output != "" {
					simplified = append(simplified, output)
				}

				output = ""
			}

			simplified = append(simplified, fmt.Sprintf("Exit Code: %d", e.ExitCode))
		default:
			return []string{}, fmt.Errorf("unknown shell event")
		}
	}

	return simplified, nil
}
