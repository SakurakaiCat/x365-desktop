export namespace main {

	export class StatusDTO {
	    running: boolean;
	    listenAddr: string;
	    currentLabel: string;
	    currentPath: string;
	    currentServer: string;
	    connectedId: string;
	    sysProxyOn: boolean;
	    tunMode: boolean;
	    tunRunning: boolean;

	    static createFrom(source: any = {}) {
	        return new StatusDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.listenAddr = source["listenAddr"];
	        this.currentLabel = source["currentLabel"];
	        this.currentPath = source["currentPath"];
	        this.currentServer = source["currentServer"];
	        this.connectedId = source["connectedId"];
	        this.sysProxyOn = source["sysProxyOn"];
	        this.tunMode = source["tunMode"];
	        this.tunRunning = source["tunRunning"];
	    }
	}

	export class ExitInfoDTO {
	    ip: string;
	    country: string;
	    countryCode: string;
	    asn: string;
	    org: string;

	    static createFrom(source: any = {}) {
	        return new ExitInfoDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.country = source["country"];
	        this.countryCode = source["countryCode"];
	        this.asn = source["asn"];
	        this.org = source["org"];
	    }
	}

	export class TrafficDTO {
	    upload: number;
	    download: number;
	    upBps: number;
	    downBps: number;

	    static createFrom(source: any = {}) {
	        return new TrafficDTO(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.upload = source["upload"];
	        this.download = source["download"];
	        this.upBps = source["upBps"];
	        this.downBps = source["downBps"];
	    }
	}

}

export namespace nodestore {

	export class Node {
	    id: string;
	    uri: string;
	    label: string;
	    server: string;
	    port: number;
	    path: string;
	    countryCode: string;
	    uuid: string;
	    sni: string;
	    pbk: string;
	    sid: string;

	    static createFrom(source: any = {}) {
	        return new Node(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.uri = source["uri"];
	        this.label = source["label"];
	        this.server = source["server"];
	        this.port = source["port"];
	        this.path = source["path"];
	        this.countryCode = source["countryCode"];
	        this.uuid = source["uuid"];
	        this.sni = source["sni"];
	        this.pbk = source["pbk"];
	        this.sid = source["sid"];
	    }
	}
	export class Settings {
	    listenAddr: string;
	    autoConnect: boolean;
	    autoSysProxy: boolean;
	    tunMode: boolean;
	    lastNodeId: string;

	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.listenAddr = source["listenAddr"];
	        this.autoConnect = source["autoConnect"];
	        this.autoSysProxy = source["autoSysProxy"];
	        this.tunMode = source["tunMode"];
	        this.lastNodeId = source["lastNodeId"];
	    }
	}

}