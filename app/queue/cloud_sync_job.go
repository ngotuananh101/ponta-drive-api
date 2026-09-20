package queue

import (
	"ponta_drive/app/jobs"
)

// CloudSyncJob is an alias for jobs.SyncS3BucketJob providing cloud synchronization queue task handling.
type CloudSyncJob = jobs.SyncS3BucketJob
