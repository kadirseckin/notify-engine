package repository

import (
	"errors"
	"github.com/lib/pq"
)

const uniqueViolation pq.ErrorCode = "23505"

func IsDuplicate(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == uniqueViolation
}

var ErrDuplicate = errors.New("duplicate entry")
