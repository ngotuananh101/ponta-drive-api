package bootstrap

import (
	"github.com/goravel/framework/contracts/queue"

	"ponta_drive/app/jobs"
)

func Jobs() []queue.Job {
	return []queue.Job{
		&jobs.SyncS3BucketJob{},
	}
}
