package frontier

import (
	"encoding/json"
	"errors"
)

type Job struct {
	URL       string `json:"url"`
	Depth     int    `json:"depth"`
	Retry     int    `json:"retry"`
	SourceURL string `json:"source_url,omitempty"`
}

func (j Job) Validate() error {
	if j.URL == "" {
		return errors.New("crawl job URL cannot be empty")
	}

	if j.Depth < 0 {
		return errors.New("crawl job depth cannot be negative")
	}

	if j.Retry < 0 {
		return errors.New("crawl job retry count cannot be negative")
	}

	return nil
}

func (j Job) Marshal() (string, error) {
	if err := j.Validate(); err != nil {
		return "", err
	}

	data, err := json.Marshal(j)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func UnmarshalJob(value string) (Job, error) {
	var job Job

	if err := json.Unmarshal([]byte(value), &job); err != nil {
		return Job{}, err
	}

	if err := job.Validate(); err != nil {
		return Job{}, err
	}

	return job, nil
}
