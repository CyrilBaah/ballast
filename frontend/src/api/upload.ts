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
// The generated class gains a convertValues helper once it has a nested
// field (caption); what the backend actually sends is the plain data.
export type UploadListItem = Omit<main.UploadListItemDTO, 'convertValues'>;

export const Start = UploadStart;
export const GetStatus = UploadGetStatus;
export const GetRecoverable = UploadGetRecoverable;
export const ConfirmRestart = UploadConfirmRestart;
export const Cancel = UploadCancel;
export const Delete = UploadDelete;
export const Retry = UploadRetry;
export const ListRecent = UploadListRecent;
