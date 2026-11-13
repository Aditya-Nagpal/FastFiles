package models

type StorageQuota struct {
	QuotaBytes    int64
	UsedBytes     int64
	ActiveBytes   int64
	TrashBytes    int64
	ReservedBytes int64
}
