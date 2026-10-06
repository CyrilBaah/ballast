import {
    UploadStart,
    UploadGetStatus,
    UploadGetRecoverable,
    UploadConfirmRestart,
    UploadCancel,
    UploadDelete,
    UploadRetry,
    UploadListRecent,
} from '../../wailsjs/go/main/App';
import type { main } from '../../wailsjs/go/models';

export type UploadStatus = main.UploadStatusDTO;
export type RecoverableUpload = main.RecoverableUploadDTO;
export type UploadListItem = main.UploadListItemDTO;

export const Start = UploadStart;
export const GetStatus = UploadGetStatus;
export const GetRecoverable = UploadGetRecoverable;
export const ConfirmRestart = UploadConfirmRestart;
export const Cancel = UploadCancel;
export const Delete = UploadDelete;
export const Retry = UploadRetry;
export const ListRecent = UploadListRecent;
