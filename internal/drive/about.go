package drive

import (
	"context"
	"fmt"

	drivev3 "google.golang.org/api/drive/v3"
)

// StorageQuota is the account's Drive storage usage (data-model.md's
// StorageQuota view entity) -- fetched fresh each session, never persisted.
type StorageQuota struct {
	UsageBytes int64 `json:"usageBytes"`
	// LimitBytes is omitted for unlimited-storage accounts (Google's
	// about.get response omits storageQuota.limit in that case) -- the
	// frontend treats an absent value as "unlimited" rather than dividing
	// by zero (data-model.md).
	LimitBytes *int64 `json:"limitBytes,omitempty"`
}

// GetStorageQuota calls Drive's about.get (fields=storageQuota) once
// (research.md §8), needing no scope beyond the drive.metadata.readonly
// scope this app already requests.
func GetStorageQuota(ctx context.Context, svc *drivev3.Service) (StorageQuota, error) {
	about, err := svc.About.Get().Fields("storageQuota").Context(ctx).Do()
	if err != nil {
		return StorageQuota{}, fmt.Errorf("drive: get storage quota: %w", err)
	}
	quota := StorageQuota{UsageBytes: about.StorageQuota.Usage}
	if about.StorageQuota.Limit > 0 {
		limit := about.StorageQuota.Limit
		quota.LimitBytes = &limit
	}
	return quota, nil
}
