export namespace appsettings {
	
	export class Settings {
	    gid: Record<string, string>;
	    zalo: Record<string, string>;
	    reminder: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gid = source["gid"];
	        this.zalo = source["zalo"];
	        this.reminder = source["reminder"];
	    }
	}

}

export namespace main {
	
	export class ZaloJob {
	    po: string;
	    system: string;
	    customerCode: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ZaloJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.po = source["po"];
	        this.system = source["system"];
	        this.customerCode = source["customerCode"];
	        this.message = source["message"];
	    }
	}

}

