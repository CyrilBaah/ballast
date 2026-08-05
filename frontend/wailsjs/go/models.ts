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

}

export namespace main {
	
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

