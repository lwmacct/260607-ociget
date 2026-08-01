package imagestore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

func (s *Store) withImageLock(ctx context.Context, imageID string, fn func() (*imageIndex, error)) (*imageIndex, error) {
	file, err := os.OpenFile(filepath.Join(s.dir, "locks", imageID[7:]+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open image build lock: %w", err)
	}
	defer file.Close()
	for {
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if err != unix.EWOULDBLOCK && err != unix.EAGAIN {
			return nil, fmt.Errorf("lock image build: %w", err)
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)
	return fn()
}
