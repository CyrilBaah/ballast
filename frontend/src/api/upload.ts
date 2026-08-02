import { UploadStart, UploadGetStatus } from '../../wailsjs/go/main/App';
import type { main } from '../../wailsjs/go/models';

export type UploadStatus = main.UploadStatusDTO;

export const Start = UploadStart;
export const GetStatus = UploadGetStatus;
