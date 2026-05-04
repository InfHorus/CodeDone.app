export namespace config {
	
	export class Config {
	    provider: string;
	    model: string;
	    cmModel: string;
	    implementerModel: string;
	    apiKey: string;
	    keys: Record<string, string>;
	    cmCount: number;
	    maxAgents: number;
	    agentTimeout: number;
	    maxTokens: number;
	    enableTemperature: boolean;
	    temperature: number;
	    enableFinalizer: boolean;
	    autoCreateBranch: boolean;
	    requireCleanTree: boolean;
	    autoCommit: boolean;
	    branchPrefix: string;
	    gitPath: string;
	    lastWorkDir: string;
	    showTimestamps: boolean;
	    autoScroll: boolean;
	    theme: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.cmModel = source["cmModel"];
	        this.implementerModel = source["implementerModel"];
	        this.apiKey = source["apiKey"];
	        this.keys = source["keys"];
	        this.cmCount = source["cmCount"];
	        this.maxAgents = source["maxAgents"];
	        this.agentTimeout = source["agentTimeout"];
	        this.maxTokens = source["maxTokens"];
	        this.enableTemperature = source["enableTemperature"];
	        this.temperature = source["temperature"];
	        this.enableFinalizer = source["enableFinalizer"];
	        this.autoCreateBranch = source["autoCreateBranch"];
	        this.requireCleanTree = source["requireCleanTree"];
	        this.autoCommit = source["autoCommit"];
	        this.branchPrefix = source["branchPrefix"];
	        this.gitPath = source["gitPath"];
	        this.lastWorkDir = source["lastWorkDir"];
	        this.showTimestamps = source["showTimestamps"];
	        this.autoScroll = source["autoScroll"];
	        this.theme = source["theme"];
	    }
	}

}

export namespace main {
	
	export class Message {
	    id: string;
	    role: string;
	    actorId?: string;
	    label?: string;
	    phase?: string;
	    ticketId?: string;
	    status?: string;
	    content: string;
	    // Go type: time
	    time: any;
	    done: boolean;
	    ticketIndex?: number;
	    ticketTotal?: number;
	    ticketTitle?: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.actorId = source["actorId"];
	        this.label = source["label"];
	        this.phase = source["phase"];
	        this.ticketId = source["ticketId"];
	        this.status = source["status"];
	        this.content = source["content"];
	        this.time = this.convertValues(source["time"], null);
	        this.done = source["done"];
	        this.ticketIndex = source["ticketIndex"];
	        this.ticketTotal = source["ticketTotal"];
	        this.ticketTitle = source["ticketTitle"];
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
	export class PendingQuestion {
	    id: string;
	    prompt: string;
	    rationale: string;
	
	    static createFrom(source: any = {}) {
	        return new PendingQuestion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.prompt = source["prompt"];
	        this.rationale = source["rationale"];
	    }
	}
	export class PendingQuestionsPayload {
	    sessionId: string;
	    summary: string;
	    questions: PendingQuestion[];
	
	    static createFrom(source: any = {}) {
	        return new PendingQuestionsPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.summary = source["summary"];
	        this.questions = this.convertValues(source["questions"], PendingQuestion);
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
	export class Session {
	    id: string;
	    status: string;
	    mode: string;
	    branch: string;
	    workDir: string;
	    workspace: string;
	    messages: Message[];
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.status = source["status"];
	        this.mode = source["mode"];
	        this.branch = source["branch"];
	        this.workDir = source["workDir"];
	        this.workspace = source["workspace"];
	        this.messages = this.convertValues(source["messages"], Message);
	        this.createdAt = this.convertValues(source["createdAt"], null);
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
	export class TicketSummary {
	    id: string;
	    title: string;
	    status: string;
	    assignedTo?: string;
	    assignedLabel?: string;
	    summary: string;
	    acceptanceCriteria: string[];
	    constraints: string[];
	    priority: string;
	    scopeClass: string;
	    parentId?: string;
	
	    static createFrom(source: any = {}) {
	        return new TicketSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.assignedTo = source["assignedTo"];
	        this.assignedLabel = source["assignedLabel"];
	        this.summary = source["summary"];
	        this.acceptanceCriteria = source["acceptanceCriteria"];
	        this.constraints = source["constraints"];
	        this.priority = source["priority"];
	        this.scopeClass = source["scopeClass"];
	        this.parentId = source["parentId"];
	    }
	}

}

