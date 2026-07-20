package frontier

import "encoding/json"

type failedJob struct {
	Job   Job    `json:"job"`
	Error string `json:"error"`
}

func jsonFailure(
	job Job,
	failureMessage string,
) (string, error) {
	data, err := json.Marshal(failedJob{
		Job:   job,
		Error: failureMessage,
	})
	if err != nil {
		return "", err
	}

	return string(data), nil
}
