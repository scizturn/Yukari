package queue

import (
	"encoding/json"

	"github.com/kyou-id/yukari/internal/domain"
)

func EncodeWinbackJob(job domain.WinbackJob) (string, error) {
	payload, err := json.Marshal(job)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
