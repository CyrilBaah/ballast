import {
    CaptionsAnswerModelDownload,
    CaptionsGetJob,
    CaptionsGetSettings,
    CaptionsShowLocalCopy,
} from '../../wailsjs/go/main/App';
import type { events, main } from '../../wailsjs/go/models';

// The generated classes are what the backend sends as plain data.
export type CaptionJob = Omit<events.CaptionJob, 'convertValues'>;
export type CaptionSettings = main.CaptionSettingsDTO;

export const GetSettings = CaptionsGetSettings;
export const AnswerModelDownload = CaptionsAnswerModelDownload;
export const GetJob = CaptionsGetJob;
export const ShowLocalCopy = CaptionsShowLocalCopy;
