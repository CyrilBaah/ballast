export namespace drive {
	
	export class Folder {
	    id: string;
	    name: string;
	    hasChildren: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Folder(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.hasChildren = source["hasChildren"];
	    }
	}
	export class StorageQuota {
	    usageBytes: number;
	    limitBytes?: number;
	
	    static createFrom(source: any = {}) {
	        return new StorageQuota(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.usageBytes = source["usageBytes"];
	        this.limitBytes = source["limitBytes"];
	    }
	}

}

export namespace events {
	
	export class AuthStatus {
	    signedIn: boolean;
	    email?: string;
	    name?: string;
	    pictureUrl?: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.signedIn = source["signedIn"];
	        this.email = source["email"];
	        this.name = source["name"];
	        this.pictureUrl = source["pictureUrl"];
	    }
	}
	export class CaptionJob {
	    uploadId: number;
	    status: string;
	    phase?: string;
	    progressPercent: number;
	    language: string;
	    driveFileName?: string;
	    driveFileLink?: string;
	    localCopyPath?: string;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new CaptionJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uploadId = source["uploadId"];
	        this.status = source["status"];
	        this.phase = source["phase"];
	        this.progressPercent = source["progressPercent"];
	        this.language = source["language"];
	        this.driveFileName = source["driveFileName"];
	        this.driveFileLink = source["driveFileLink"];
	        this.localCopyPath = source["localCopyPath"];
	        this.note = source["note"];
	    }
	}
	export class SummaryJob {
	    uploadId: number;
	    status: string;
	    phase?: string;
	    progressPercent: number;
	    driveFileName?: string;
	    driveFileLink?: string;
	    localCopyPath?: string;
	    note?: string;
	    canRetry: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SummaryJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uploadId = source["uploadId"];
	        this.status = source["status"];
	        this.phase = source["phase"];
	        this.progressPercent = source["progressPercent"];
	        this.driveFileName = source["driveFileName"];
	        this.driveFileLink = source["driveFileLink"];
	        this.localCopyPath = source["localCopyPath"];
	        this.note = source["note"];
	        this.canRetry = source["canRetry"];
	    }
	}

}

export namespace main {
	
	export class CaptionSettingsDTO {
	    enabled: boolean;
	    language: string;
	    available: boolean;
	    unavailableReason?: string;
	    modelConsent: string;
	    modelDownloaded: boolean;
	    modelSizeBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new CaptionSettingsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.language = source["language"];
	        this.available = source["available"];
	        this.unavailableReason = source["unavailableReason"];
	        this.modelConsent = source["modelConsent"];
	        this.modelDownloaded = source["modelDownloaded"];
	        this.modelSizeBytes = source["modelSizeBytes"];
	    }
	}
	export class LocalFileRef {
	    path: string;
	    name: string;
	    sizeBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new LocalFileRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.sizeBytes = source["sizeBytes"];
	    }
	}
	export class RecoverableUploadDTO {
	    id: number;
	    localPath: string;
	    fileName: string;
	    status: string;
	    bytesSent: number;
	    totalBytes: number;
	    awaitingConfirmationReason?: string;
	
	    static createFrom(source: any = {}) {
	        return new RecoverableUploadDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.localPath = source["localPath"];
	        this.fileName = source["fileName"];
	        this.status = source["status"];
	        this.bytesSent = source["bytesSent"];
	        this.totalBytes = source["totalBytes"];
	        this.awaitingConfirmationReason = source["awaitingConfirmationReason"];
	    }
	}
	export class SummarySettingsDTO {
	    enabled: boolean;
	    available: boolean;
	    unavailableReason?: string;
	    modelConsent: string;
	    modelDownloaded: boolean;
	    modelSizeBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new SummarySettingsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.available = source["available"];
	        this.unavailableReason = source["unavailableReason"];
	        this.modelConsent = source["modelConsent"];
	        this.modelDownloaded = source["modelDownloaded"];
	        this.modelSizeBytes = source["modelSizeBytes"];
	    }
	}
	export class UploadListItemDTO {
	    id: number;
	    fileName: string;
	    driveFolderName: string;
	    status: string;
	    bytesSent: number;
	    totalBytes: number;
	    driveFileLink?: string;
	    failureReason?: string;
	    startedAt: string;
	    caption?: events.CaptionJob;
	    summary?: events.SummaryJob;
	
	    static createFrom(source: any = {}) {
	        return new UploadListItemDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.fileName = source["fileName"];
	        this.driveFolderName = source["driveFolderName"];
	        this.status = source["status"];
	        this.bytesSent = source["bytesSent"];
	        this.totalBytes = source["totalBytes"];
	        this.driveFileLink = source["driveFileLink"];
	        this.failureReason = source["failureReason"];
	        this.startedAt = source["startedAt"];
	        this.caption = this.convertValues(source["caption"], events.CaptionJob);
	        this.summary = this.convertValues(source["summary"], events.SummaryJob);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UploadStatusDTO {
	    status: string;
	    bytesSent: number;
	    totalBytes: number;
	    driveFileLink?: string;
	    failureReason?: string;
	    awaitingConfirmationReason?: string;
	
	    static createFrom(source: any = {}) {
	        return new UploadStatusDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.bytesSent = source["bytesSent"];
	        this.totalBytes = source["totalBytes"];
	        this.driveFileLink = source["driveFileLink"];
	        this.failureReason = source["failureReason"];
	        this.awaitingConfirmationReason = source["awaitingConfirmationReason"];
	    }
	}

}

