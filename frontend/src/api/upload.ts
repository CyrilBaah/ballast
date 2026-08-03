import {
    UploadStart,
    UploadGetStatus,
    UploadGetRecoverable,
    UploadConfirmRestart,
    UploadCancel,
} from '../../wailsjs/go/main/App';
import type { main } from '../../wailsjs/go/models';

export type UploadStatus = main.UploadStatusDTO;
export type RecoverableUpload = main.RecoverableUploadDTO;

export const Start = UploadStart;
export const GetStatus = UploadGetStatus;
export const GetRecoverable = UploadGetRecoverable;
export const ConfirmRestart = UploadConfirmRestart;
export const Cancel = UploadCancel;
