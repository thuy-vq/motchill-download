export namespace main {
	
	export class Episode {
	    id: string;
	    name: string;
	    number: number;
	    pageUrl: string;
	    streamUrl?: string;
	    current: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Episode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.number = source["number"];
	        this.pageUrl = source["pageUrl"];
	        this.streamUrl = source["streamUrl"];
	        this.current = source["current"];
	    }
	}
	export class MediaStream {
	    url: string;
	    kind: string;
	    server: string;
	
	    static createFrom(source: any = {}) {
	        return new MediaStream(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.kind = source["kind"];
	        this.server = source["server"];
	    }
	}
	export class AnalysisResult {
	    title: string;
	    pageUrl: string;
	    streams: MediaStream[];
	    episodes: Episode[];
	    htmlBytes: number;
	    sourceLabel: string;
	
	    static createFrom(source: any = {}) {
	        return new AnalysisResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.pageUrl = source["pageUrl"];
	        this.streams = this.convertValues(source["streams"], MediaStream);
	        this.episodes = this.convertValues(source["episodes"], Episode);
	        this.htmlBytes = source["htmlBytes"];
	        this.sourceLabel = source["sourceLabel"];
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
	export class DownloadItem {
	    id: string;
	    name: string;
	    number: number;
	    pageUrl: string;
	    streamUrl?: string;
	    title?: string;
	    outputDir?: string;
	
	    static createFrom(source: any = {}) {
	        return new DownloadItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.number = source["number"];
	        this.pageUrl = source["pageUrl"];
	        this.streamUrl = source["streamUrl"];
	        this.title = source["title"];
	        this.outputDir = source["outputDir"];
	    }
	}
	export class DownloadRequest {
	    title: string;
	    outputDir: string;
	    preferredServer: string;
	    items: DownloadItem[];
	    skipExisting: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DownloadRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.outputDir = source["outputDir"];
	        this.preferredServer = source["preferredServer"];
	        this.items = this.convertValues(source["items"], DownloadItem);
	        this.skipExisting = source["skipExisting"];
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
	
	export class FFmpegStatus {
	    ready: boolean;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new FFmpegStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.path = source["path"];
	    }
	}
	export class InitialState {
	    lastOutputDir: string;
	    ffmpegReady: boolean;
	    ffmpegPath: string;
	    platform: string;
	
	    static createFrom(source: any = {}) {
	        return new InitialState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lastOutputDir = source["lastOutputDir"];
	        this.ffmpegReady = source["ffmpegReady"];
	        this.ffmpegPath = source["ffmpegPath"];
	        this.platform = source["platform"];
	    }
	}
	
	export class SourceDocument {
	    path: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new SourceDocument(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	    }
	}

}

