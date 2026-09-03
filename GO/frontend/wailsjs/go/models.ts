export namespace appsettings {
	
	export class Settings {
	    gid: Record<string, string>;
	    zalo: Record<string, string>;
	    reminder: Record<string, string>;
	    haravan: Record<string, string>;
	    misa: Record<string, string>;
	    misa_routing: Record<string, string>;
	    warehouse: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gid = source["gid"];
	        this.zalo = source["zalo"];
	        this.reminder = source["reminder"];
	        this.haravan = source["haravan"];
	        this.misa = source["misa"];
	        this.misa_routing = source["misa_routing"];
	        this.warehouse = source["warehouse"];
	    }
	}

}

export namespace main {
	
	export class MisaPushJob {
	    po: string;
	    routeKey: string;
	    branch: string;
	    excelRows: number[];
	
	    static createFrom(source: any = {}) {
	        return new MisaPushJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.po = source["po"];
	        this.routeKey = source["routeKey"];
	        this.branch = source["branch"];
	        this.excelRows = source["excelRows"];
	    }
	}
	export class MisaRouteInfo {
	    key: string;
	    label: string;
	    branch: string;
	
	    static createFrom(source: any = {}) {
	        return new MisaRouteInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.branch = source["branch"];
	    }
	}
	export class MisaRouteInput {
	    system: string;
	    customerCode: string;
	    shipTo: string;
	
	    static createFrom(source: any = {}) {
	        return new MisaRouteInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.system = source["system"];
	        this.customerCode = source["customerCode"];
	        this.shipTo = source["shipTo"];
	    }
	}
	export class TMDTComboEntry {
	    key: string;
	    product: string;
	    variant: string;
	    combo: string;
	    tp: string[];
	    sl: string[];
	
	    static createFrom(source: any = {}) {
	        return new TMDTComboEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.product = source["product"];
	        this.variant = source["variant"];
	        this.combo = source["combo"];
	        this.tp = source["tp"];
	        this.sl = source["sl"];
	    }
	}
	export class WarehouseInfo {
	    key: string;
	    label: string;
	    code: string;
	    default: string;
	
	    static createFrom(source: any = {}) {
	        return new WarehouseInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.code = source["code"];
	        this.default = source["default"];
	    }
	}
	export class ZaloJob {
	    po: string;
	    system: string;
	    customerCode: string;
	    message: string;
	    displayLabel: string;
	
	    static createFrom(source: any = {}) {
	        return new ZaloJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.po = source["po"];
	        this.system = source["system"];
	        this.customerCode = source["customerCode"];
	        this.message = source["message"];
	        this.displayLabel = source["displayLabel"];
	    }
	}

}

