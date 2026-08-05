import { DriveListFolders, DriveGetStorageQuota } from '../../wailsjs/go/main/App';
import type { drive } from '../../wailsjs/go/models';

export type DriveFolder = drive.Folder;
export type StorageQuota = drive.StorageQuota;

export const ListFolders = DriveListFolders;
export const GetStorageQuota = DriveGetStorageQuota;
