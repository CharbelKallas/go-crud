package repositories

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestRetryOnDeadlock(t *testing.T) {
	deadlock := fmt.Errorf("rotate: %w", &mysql.MySQLError{Number: mysqlErrDeadlock})

	calls := 0
	err := retryOnDeadlock(func() error {
		calls++
		if calls < 3 {
			return deadlock
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err = %v after %d calls, want success on the 3rd", err, calls)
	}

	calls = 0
	err = retryOnDeadlock(func() error { calls++; return deadlock })
	if !errors.Is(err, deadlock) || calls != 3 {
		t.Fatalf("err = %v after %d calls, want the deadlock after 3", err, calls)
	}

	calls = 0
	other := errors.New("boom")
	if err := retryOnDeadlock(func() error { calls++; return other }); err != other || calls != 1 {
		t.Fatalf("err = %v after %d calls, want no retry for other errors", err, calls)
	}
}
