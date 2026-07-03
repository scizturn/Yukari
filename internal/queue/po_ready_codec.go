package queue

import (
	"encoding/json"

	"github.com/kyou-id/yukari/internal/domain"
)

func EncodePoReadyJob(job domain.PoReadyJob) (string, error) {
	payload, err := json.Marshal(job)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
