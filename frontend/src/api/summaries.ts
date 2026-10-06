import {
    SummariesAnswerModelDownload,
    SummariesGetJob,
    SummariesGetSettings,
    SummariesRetry,
    SummariesSetEnabled,
    SummariesShowLocalCopy,
} from '../../wailsjs/go/main/App';
import type { events, main } from '../../wailsjs/go/models';

// The generated classes are what the backend sends as plain data.
export type SummaryJob = Omit<events.SummaryJob, 'convertValues'>;
export type SummarySettings = main.SummarySettingsDTO;

export const GetSettings = SummariesGetSettings;
export const SetEnabled = SummariesSetEnabled;
export const AnswerModelDownload = SummariesAnswerModelDownload;
export const GetJob = SummariesGetJob;
export const Retry = SummariesRetry;
export const ShowLocalCopy = SummariesShowLocalCopy;
