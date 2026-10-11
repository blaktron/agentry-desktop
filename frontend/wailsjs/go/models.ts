export namespace build {
	
	export class AuthorRow {
	    name: string;
	    email: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthorRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.email = source["email"];
	    }
	}
	export class SectionProblem {
	    section: string;
	    problem: string;
	
	    static createFrom(source: any = {}) {
	        return new SectionProblem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.section = source["section"];
	        this.problem = source["problem"];
	    }
	}
	export class CheckResult {
	    yaml: string;
	    fragments: {[key: string]: string};
	    problems: {[key: string]: string};
	    first?: SectionProblem;
	    warnings: string[];
	    public?: string;
	
	    static createFrom(source: any = {}) {
	        return new CheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.yaml = source["yaml"];
	        this.fragments = source["fragments"];
	        this.problems = source["problems"];
	        this.first = this.convertValues(source["first"], SectionProblem);
	        this.warnings = source["warnings"];
	        this.public = source["public"];
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
	export class ConnectRow {
	    secret: string;
	    help: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.secret = source["secret"];
	        this.help = source["help"];
	    }
	}
	export class DependencyRow {
	    ref: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new DependencyRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.version = source["version"];
	    }
	}
	export class DiffLine {
	    op: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new DiffLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.op = source["op"];
	        this.text = source["text"];
	    }
	}
	export class DiffFile {
	    path: string;
	    lines: DiffLine[];
	    same: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DiffFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.lines = this.convertValues(source["lines"], DiffLine);
	        this.same = source["same"];
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
	
	export class EnvRow {
	    name: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new EnvRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class GuideModel {
	    overview: string;
	    prepare: string[];
	    files: string;
	    task: string;
	    connect: ConnectRow[];
	
	    static createFrom(source: any = {}) {
	        return new GuideModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.overview = source["overview"];
	        this.prepare = source["prepare"];
	        this.files = source["files"];
	        this.task = source["task"];
	        this.connect = this.convertValues(source["connect"], ConnectRow);
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
	export class IntakeRow {
	    id: string;
	    prompt: string;
	    type: string;
	    required: boolean;
	    choices: string[];
	
	    static createFrom(source: any = {}) {
	        return new IntakeRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.prompt = source["prompt"];
	        this.type = source["type"];
	        this.required = source["required"];
	        this.choices = source["choices"];
	    }
	}
	export class TokenFileRow {
	    path: string;
	    template: string;
	
	    static createFrom(source: any = {}) {
	        return new TokenFileRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.template = source["template"];
	    }
	}
	export class OAuthRow {
	    issuer: string;
	    scopes: string[];
	    files: TokenFileRow[];
	    env: EnvRow[];
	
	    static createFrom(source: any = {}) {
	        return new OAuthRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.issuer = source["issuer"];
	        this.scopes = source["scopes"];
	        this.files = this.convertValues(source["files"], TokenFileRow);
	        this.env = this.convertValues(source["env"], EnvRow);
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
	export class MCPServerRow {
	    source: string;
	    package: string;
	    version: string;
	    url: string;
	    auth: string;
	    scopes: string[];
	    oauth?: OAuthRow;
	    tools: string[];
	    secrets: string[];
	    egress: string[];
	    env: EnvRow[];
	
	    static createFrom(source: any = {}) {
	        return new MCPServerRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.package = source["package"];
	        this.version = source["version"];
	        this.url = source["url"];
	        this.auth = source["auth"];
	        this.scopes = source["scopes"];
	        this.oauth = this.convertValues(source["oauth"], OAuthRow);
	        this.tools = source["tools"];
	        this.secrets = source["secrets"];
	        this.egress = source["egress"];
	        this.env = this.convertValues(source["env"], EnvRow);
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
	export class Model {
	    name: string;
	    namespace: string;
	    modes: string[];
	    version: string;
	    summary: string;
	    license: string;
	    authors: AuthorRow[];
	    homepage?: string;
	    tags: string[];
	    instructionFile: string;
	    instructions: string;
	    intake: IntakeRow[];
	    opening: string;
	    idleTimeoutSeconds: number;
	    guide: GuideModel;
	    runtimeModels: string[];
	    runtimeInterfaces: string[];
	    egressMode: string;
	    egressHosts: string[];
	    fsRead: string;
	    fsReadPaths: string[];
	    fsWrite: string;
	    fsWritePaths: string[];
	    execCommands: string[];
	    secrets: string[];
	    requiredSecrets: string[];
	    dataAccess: string[];
	    mcpServers: MCPServerRow[];
	    knowledgePrompt: string;
	    knowledgeRequired: boolean;
	    knowledgeShips: string[];
	    dependencies: DependencyRow[];
	    publishChannel: string;
	    publishInclude: string[];
	    hadComments: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Model(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.modes = source["modes"];
	        this.version = source["version"];
	        this.summary = source["summary"];
	        this.license = source["license"];
	        this.authors = this.convertValues(source["authors"], AuthorRow);
	        this.homepage = source["homepage"];
	        this.tags = source["tags"];
	        this.instructionFile = source["instructionFile"];
	        this.instructions = source["instructions"];
	        this.intake = this.convertValues(source["intake"], IntakeRow);
	        this.opening = source["opening"];
	        this.idleTimeoutSeconds = source["idleTimeoutSeconds"];
	        this.guide = this.convertValues(source["guide"], GuideModel);
	        this.runtimeModels = source["runtimeModels"];
	        this.runtimeInterfaces = source["runtimeInterfaces"];
	        this.egressMode = source["egressMode"];
	        this.egressHosts = source["egressHosts"];
	        this.fsRead = source["fsRead"];
	        this.fsReadPaths = source["fsReadPaths"];
	        this.fsWrite = source["fsWrite"];
	        this.fsWritePaths = source["fsWritePaths"];
	        this.execCommands = source["execCommands"];
	        this.secrets = source["secrets"];
	        this.requiredSecrets = source["requiredSecrets"];
	        this.dataAccess = source["dataAccess"];
	        this.mcpServers = this.convertValues(source["mcpServers"], MCPServerRow);
	        this.knowledgePrompt = source["knowledgePrompt"];
	        this.knowledgeRequired = source["knowledgeRequired"];
	        this.knowledgeShips = source["knowledgeShips"];
	        this.dependencies = this.convertValues(source["dependencies"], DependencyRow);
	        this.publishChannel = source["publishChannel"];
	        this.publishInclude = source["publishInclude"];
	        this.hadComments = source["hadComments"];
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
	
	export class PublishResult {
	    version: string;
	    status: string;
	    digest: string;
	    detail?: string;
	    refusal?: string;
	    account?: string;
	    findingCounts?: {[key: string]: number};
	    visibility?: string;
	    visibilityNote?: string;
	    settingsUrl?: string;
	
	    static createFrom(source: any = {}) {
	        return new PublishResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.status = source["status"];
	        this.digest = source["digest"];
	        this.detail = source["detail"];
	        this.refusal = source["refusal"];
	        this.account = source["account"];
	        this.findingCounts = source["findingCounts"];
	        this.visibility = source["visibility"];
	        this.visibilityNote = source["visibilityNote"];
	        this.settingsUrl = source["settingsUrl"];
	    }
	}
	
	export class SummaryServer {
	    ref: string;
	    tools: string[];
	    hosts: SummaryHost[];
	    keys: string[];
	    config: string[];
	    remote: boolean;
	    signIn: string[];
	
	    static createFrom(source: any = {}) {
	        return new SummaryServer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.tools = source["tools"];
	        this.hosts = this.convertValues(source["hosts"], SummaryHost);
	        this.keys = source["keys"];
	        this.config = source["config"];
	        this.remote = source["remote"];
	        this.signIn = source["signIn"];
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
	export class SummaryHost {
	    host: string;
	    unnamed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SummaryHost(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.unnamed = source["unnamed"];
	    }
	}
	export class SummaryKey {
	    name: string;
	    help: string;
	    usedBy: string[];
	    optional: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SummaryKey(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.help = source["help"];
	        this.usedBy = source["usedBy"];
	        this.optional = source["optional"];
	    }
	}
	export class Summary {
	    name: string;
	    summary: string;
	    overview: string;
	    keys: SummaryKey[];
	    websites: SummaryHost[];
	    servers: SummaryServer[];
	    grants: string[];
	    knowledge: string[];
	    anyUnnamed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.summary = source["summary"];
	        this.overview = source["overview"];
	        this.keys = this.convertValues(source["keys"], SummaryKey);
	        this.websites = this.convertValues(source["websites"], SummaryHost);
	        this.servers = this.convertValues(source["servers"], SummaryServer);
	        this.grants = source["grants"];
	        this.knowledge = source["knowledge"];
	        this.anyUnnamed = source["anyUnnamed"];
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
	
	
	

}

export namespace dockerimages {
	
	export class Report {
	    removed: string[];
	    failed: string[];
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.removed = source["removed"];
	        this.failed = source["failed"];
	    }
	}

}

export namespace draft {
	
	export class Line {
	    tone: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new Line(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tone = source["tone"];
	        this.text = source["text"];
	    }
	}

}

export namespace drafter {
	
	export class Match {
	    ref: string;
	    why: string;
	
	    static createFrom(source: any = {}) {
	        return new Match(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.why = source["why"];
	    }
	}
	export class WithheldFile {
	    path: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new WithheldFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.reason = source["reason"];
	    }
	}
	export class PackRead {
	    name: string;
	    digest: string;
	    pages: number;
	    bytes: number;
	    scan: string;
	    withheld: WithheldFile[];
	    requests?: number;
	
	    static createFrom(source: any = {}) {
	        return new PackRead(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.digest = source["digest"];
	        this.pages = source["pages"];
	        this.bytes = source["bytes"];
	        this.scan = source["scan"];
	        this.withheld = this.convertValues(source["withheld"], WithheldFile);
	        this.requests = source["requests"];
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
	export class Source {
	    url: string;
	    used: string;
	    withheld?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Source(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.used = source["used"];
	        this.withheld = source["withheld"];
	    }
	}
	
	export class WithheldSource {
	    tool: string;
	    url: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new WithheldSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tool = source["tool"];
	        this.url = source["url"];
	        this.reason = source["reason"];
	    }
	}

}

export namespace install {
	
	export class CodingAgentExport {
	    target: string;
	    dir: string;
	    needsDir: boolean;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CodingAgentExport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.dir = source["dir"];
	        this.needsDir = source["needsDir"];
	        this.enabled = source["enabled"];
	    }
	}
	export class Item {
	    registry: string;
	    namespace: string;
	    name: string;
	    target: string;
	    dir: string;
	    version: string;
	    // Go type: time
	    installedAt: any;
	    capabilities: installed.CapSnapshot;
	    managed?: boolean;
	    source?: string;
	    digest?: string;
	    origin: string;
	    present: boolean;
	    local: boolean;
	    manageable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registry = source["registry"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.target = source["target"];
	        this.dir = source["dir"];
	        this.version = source["version"];
	        this.installedAt = this.convertValues(source["installedAt"], null);
	        this.capabilities = this.convertValues(source["capabilities"], installed.CapSnapshot);
	        this.managed = source["managed"];
	        this.source = source["source"];
	        this.digest = source["digest"];
	        this.origin = source["origin"];
	        this.present = source["present"];
	        this.local = source["local"];
	        this.manageable = source["manageable"];
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
	export class Paths {
	    dest: string;
	    managed: boolean;
	    managedRoot?: string;
	    agentsMd?: string;
	    releaseDir?: string;
	
	    static createFrom(source: any = {}) {
	        return new Paths(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dest = source["dest"];
	        this.managed = source["managed"];
	        this.managedRoot = source["managedRoot"];
	        this.agentsMd = source["agentsMd"];
	        this.releaseDir = source["releaseDir"];
	    }
	}
	export class ShippedPack {
	    name: string;
	    bytes?: number;
	
	    static createFrom(source: any = {}) {
	        return new ShippedPack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.bytes = source["bytes"];
	    }
	}
	export class Plan {
	    id: string;
	    registry: string;
	    namespace: string;
	    name: string;
	    version: string;
	    modes: string[];
	    summary: string;
	    digest: string;
	    yanked: boolean;
	    attestation: trust.AttestationResult;
	    unsigned: boolean;
	    unscanned: boolean;
	    capabilities: installed.CapSnapshot;
	    declaredSecrets: string[];
	    optionalSecrets: string[];
	    shippedKnowledge: ShippedPack[];
	    mcpServers: mcpview.Server[];
	    needsCapabilityConfirm: boolean;
	    prevVersion?: string;
	    diff?: manifest.CapabilityDiff;
	    requiresWidenedConfirm: boolean;
	    vulnerabilities: mcpvuln.Status[];
	    vulnerabilityLine?: string;
	    vulnerabilityRefusal?: string;
	    target: string;
	    paths: Paths;
	    targetWarning?: string;
	    secrets?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Plan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.registry = source["registry"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.modes = source["modes"];
	        this.summary = source["summary"];
	        this.digest = source["digest"];
	        this.yanked = source["yanked"];
	        this.attestation = this.convertValues(source["attestation"], trust.AttestationResult);
	        this.unsigned = source["unsigned"];
	        this.unscanned = source["unscanned"];
	        this.capabilities = this.convertValues(source["capabilities"], installed.CapSnapshot);
	        this.declaredSecrets = source["declaredSecrets"];
	        this.optionalSecrets = source["optionalSecrets"];
	        this.shippedKnowledge = this.convertValues(source["shippedKnowledge"], ShippedPack);
	        this.mcpServers = this.convertValues(source["mcpServers"], mcpview.Server);
	        this.needsCapabilityConfirm = source["needsCapabilityConfirm"];
	        this.prevVersion = source["prevVersion"];
	        this.diff = this.convertValues(source["diff"], manifest.CapabilityDiff);
	        this.requiresWidenedConfirm = source["requiresWidenedConfirm"];
	        this.vulnerabilities = this.convertValues(source["vulnerabilities"], mcpvuln.Status);
	        this.vulnerabilityLine = source["vulnerabilityLine"];
	        this.vulnerabilityRefusal = source["vulnerabilityRefusal"];
	        this.target = source["target"];
	        this.paths = this.convertValues(source["paths"], Paths);
	        this.targetWarning = source["targetWarning"];
	        this.secrets = source["secrets"];
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
	export class Result {
	    message: string;
	    dest: string;
	    secretsNote?: string;
	    warning?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	        this.dest = source["dest"];
	        this.secretsNote = source["secretsNote"];
	        this.warning = source["warning"];
	    }
	}
	
	export class UpdateInfo {
	    key: installed.Key;
	    current: string;
	    latest?: string;
	    updateAvailable: boolean;
	    error?: string;
	    manageable: boolean;
	    present: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = this.convertValues(source["key"], installed.Key);
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.updateAvailable = source["updateAvailable"];
	        this.error = source["error"];
	        this.manageable = source["manageable"];
	        this.present = source["present"];
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

}

export namespace installed {
	
	export class RemoteSnapshot {
	    auth: string;
	    scopes?: string[];
	    tools?: string[];
	    secrets?: string[];
	
	    static createFrom(source: any = {}) {
	        return new RemoteSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.auth = source["auth"];
	        this.scopes = source["scopes"];
	        this.tools = source["tools"];
	        this.secrets = source["secrets"];
	    }
	}
	export class CapSnapshot {
	    egressHosts?: string[];
	    egressNone?: boolean;
	    fsRead?: string;
	    fsReadPaths?: string[];
	    fsWrite?: string;
	    fsWritePaths?: string[];
	    execCommands?: string[];
	    secrets?: string[];
	    dataAccess?: string[];
	    mcpServers?: string[];
	    mcpServerEnv?: {[key: string]: any};
	    mcpRemote?: {[key: string]: RemoteSnapshot};
	    mcpServerOAuth?: {[key: string]: manifest.MCPOAuth};
	    knowledge?: string[];
	
	    static createFrom(source: any = {}) {
	        return new CapSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.egressHosts = source["egressHosts"];
	        this.egressNone = source["egressNone"];
	        this.fsRead = source["fsRead"];
	        this.fsReadPaths = source["fsReadPaths"];
	        this.fsWrite = source["fsWrite"];
	        this.fsWritePaths = source["fsWritePaths"];
	        this.execCommands = source["execCommands"];
	        this.secrets = source["secrets"];
	        this.dataAccess = source["dataAccess"];
	        this.mcpServers = source["mcpServers"];
	        this.mcpServerEnv = source["mcpServerEnv"];
	        this.mcpRemote = this.convertValues(source["mcpRemote"], RemoteSnapshot, true);
	        this.mcpServerOAuth = this.convertValues(source["mcpServerOAuth"], manifest.MCPOAuth, true);
	        this.knowledge = source["knowledge"];
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
	export class Key {
	    registry: string;
	    namespace: string;
	    name: string;
	    target: string;
	    dir: string;
	
	    static createFrom(source: any = {}) {
	        return new Key(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registry = source["registry"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.target = source["target"];
	        this.dir = source["dir"];
	    }
	}
	export class Record {
	    registry: string;
	    namespace: string;
	    name: string;
	    target: string;
	    dir: string;
	    version: string;
	    // Go type: time
	    installedAt: any;
	    capabilities: CapSnapshot;
	    managed?: boolean;
	    source?: string;
	    digest?: string;
	
	    static createFrom(source: any = {}) {
	        return new Record(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registry = source["registry"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.target = source["target"];
	        this.dir = source["dir"];
	        this.version = source["version"];
	        this.installedAt = this.convertValues(source["installedAt"], null);
	        this.capabilities = this.convertValues(source["capabilities"], CapSnapshot);
	        this.managed = source["managed"];
	        this.source = source["source"];
	        this.digest = source["digest"];
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

}

export namespace knowledge {
	
	export class Dropped {
	    path: string;
	    reason: string;
	    kept?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Dropped(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.reason = source["reason"];
	        this.kept = source["kept"];
	    }
	}
	export class BuildResult {
	    name: string;
	    pages: number;
	    bytes: number;
	    documents: string[];
	    brief?: boolean;
	    skipped?: string[];
	    dropped?: Dropped[];
	    withoutWeb?: string;
	    searched?: boolean;
	    stopped?: string;
	    key: string;
	    agentryStale?: boolean;
	    rewrapped?: string[];
	    inputTokens: number;
	    outputTokens: number;
	    costUsd: number;
	
	    static createFrom(source: any = {}) {
	        return new BuildResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.pages = source["pages"];
	        this.bytes = source["bytes"];
	        this.documents = source["documents"];
	        this.brief = source["brief"];
	        this.skipped = source["skipped"];
	        this.dropped = this.convertValues(source["dropped"], Dropped);
	        this.withoutWeb = source["withoutWeb"];
	        this.searched = source["searched"];
	        this.stopped = source["stopped"];
	        this.key = source["key"];
	        this.agentryStale = source["agentryStale"];
	        this.rewrapped = source["rewrapped"];
	        this.inputTokens = source["inputTokens"];
	        this.outputTokens = source["outputTokens"];
	        this.costUsd = source["costUsd"];
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
	
	export class Pack {
	    name: string;
	    id?: string;
	    agentryId?: string;
	    pages: number;
	    bytes: number;
	    scan: string;
	    scanState: string;
	    local: boolean;
	    inAgentry: boolean;
	    agentryReady: boolean;
	    changed: boolean;
	    where: string;
	    note?: string;
	    key?: string;
	    wrapper?: string;
	    registry?: string;
	    opensOffline: boolean;
	    shared: boolean;
	    org?: string;
	    exportable: boolean;
	    canExport: boolean;
	    updated?: string;
	    agentryUpdated?: string;
	    agentryScan?: string;
	    agentryScanState?: string;
	    agentryPages?: number;
	    agentryBytes?: number;
	
	    static createFrom(source: any = {}) {
	        return new Pack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.id = source["id"];
	        this.agentryId = source["agentryId"];
	        this.pages = source["pages"];
	        this.bytes = source["bytes"];
	        this.scan = source["scan"];
	        this.scanState = source["scanState"];
	        this.local = source["local"];
	        this.inAgentry = source["inAgentry"];
	        this.agentryReady = source["agentryReady"];
	        this.changed = source["changed"];
	        this.where = source["where"];
	        this.note = source["note"];
	        this.key = source["key"];
	        this.wrapper = source["wrapper"];
	        this.registry = source["registry"];
	        this.opensOffline = source["opensOffline"];
	        this.shared = source["shared"];
	        this.org = source["org"];
	        this.exportable = source["exportable"];
	        this.canExport = source["canExport"];
	        this.updated = source["updated"];
	        this.agentryUpdated = source["agentryUpdated"];
	        this.agentryScan = source["agentryScan"];
	        this.agentryScanState = source["agentryScanState"];
	        this.agentryPages = source["agentryPages"];
	        this.agentryBytes = source["agentryBytes"];
	    }
	}
	export class List {
	    packs: Pack[];
	    registry: string;
	    signedIn: boolean;
	    agentry?: string;
	    shared?: string;
	    problems?: string[];
	
	    static createFrom(source: any = {}) {
	        return new List(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.packs = this.convertValues(source["packs"], Pack);
	        this.registry = source["registry"];
	        this.signedIn = source["signedIn"];
	        this.agentry = source["agentry"];
	        this.shared = source["shared"];
	        this.problems = source["problems"];
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
	
	export class Page {
	    path: string;
	    title: string;
	    summary?: string;
	    markdown: string;
	    truncated?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Page(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.title = source["title"];
	        this.summary = source["summary"];
	        this.markdown = source["markdown"];
	        this.truncated = source["truncated"];
	    }
	}
	export class PageInfo {
	    path: string;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new PageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.title = source["title"];
	    }
	}
	export class Preview {
	    name: string;
	    index: Page;
	    pages: PageInfo[];
	
	    static createFrom(source: any = {}) {
	        return new Preview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.index = this.convertValues(source["index"], Page);
	        this.pages = this.convertValues(source["pages"], PageInfo);
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

}

export namespace main {
	
	export class DependencyAlertView {
	    id: string;
	    version: string;
	    ecosystem: string;
	    package: string;
	    packageVersion: string;
	    severity: string;
	    advisoryIds: string[];
	    fixedIn: string[];
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DependencyAlertView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.version = source["version"];
	        this.ecosystem = source["ecosystem"];
	        this.package = source["package"];
	        this.packageVersion = source["packageVersion"];
	        this.severity = source["severity"];
	        this.advisoryIds = source["advisoryIds"];
	        this.fixedIn = source["fixedIn"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class VersionView {
	    version: string;
	    channel: string;
	    status: string;
	    createdAt: string;
	    publishedAt?: string;
	    archiveDigest?: string;
	    yanked: boolean;
	    yankReason?: string;
	    scanStatus?: string;
	
	    static createFrom(source: any = {}) {
	        return new VersionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.channel = source["channel"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	        this.publishedAt = source["publishedAt"];
	        this.archiveDigest = source["archiveDigest"];
	        this.yanked = source["yanked"];
	        this.yankReason = source["yankReason"];
	        this.scanStatus = source["scanStatus"];
	    }
	}
	export class AgentForkOrigin {
	    namespace: string;
	    name: string;
	    version: string;
	    available: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentForkOrigin(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.available = source["available"];
	    }
	}
	export class AgentView {
	    namespace: string;
	    name: string;
	    modes: string[];
	    summary: string;
	    visibility: string;
	    quarantine?: string;
	    createdAt: string;
	    gitHttpUrl?: string;
	    forkedFrom?: AgentForkOrigin;
	    forkState?: string;
	    forkDetail?: string;
	    forkCount?: number;
	
	    static createFrom(source: any = {}) {
	        return new AgentView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.modes = source["modes"];
	        this.summary = source["summary"];
	        this.visibility = source["visibility"];
	        this.quarantine = source["quarantine"];
	        this.createdAt = source["createdAt"];
	        this.gitHttpUrl = source["gitHttpUrl"];
	        this.forkedFrom = this.convertValues(source["forkedFrom"], AgentForkOrigin);
	        this.forkState = source["forkState"];
	        this.forkDetail = source["forkDetail"];
	        this.forkCount = source["forkCount"];
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
	export class AgentDetailView {
	    agent: AgentView;
	    versions: VersionView[];
	    latest: string;
	    latestCapabilities?: {[key: string]: any};
	    latestDigest?: string;
	    latestScanStatus?: string;
	    dependencyAlerts: DependencyAlertView[];
	    canPublish: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentDetailView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agent = this.convertValues(source["agent"], AgentView);
	        this.versions = this.convertValues(source["versions"], VersionView);
	        this.latest = source["latest"];
	        this.latestCapabilities = source["latestCapabilities"];
	        this.latestDigest = source["latestDigest"];
	        this.latestScanStatus = source["latestScanStatus"];
	        this.dependencyAlerts = this.convertValues(source["dependencyAlerts"], DependencyAlertView);
	        this.canPublish = source["canPublish"];
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
	
	
	export class CLIInstallView {
	    path: string;
	    version?: string;
	    notes?: string[];
	    log: string;
	
	    static createFrom(source: any = {}) {
	        return new CLIInstallView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.version = source["version"];
	        this.notes = source["notes"];
	        this.log = source["log"];
	    }
	}
	export class CalendarFire {
	    hosted: boolean;
	    scheduleId: string;
	    name: string;
	    at: string;
	    outcome: string;
	    reason?: string;
	    runId?: string;
	    zone?: string;
	
	    static createFrom(source: any = {}) {
	        return new CalendarFire(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hosted = source["hosted"];
	        this.scheduleId = source["scheduleId"];
	        this.name = source["name"];
	        this.at = source["at"];
	        this.outcome = source["outcome"];
	        this.reason = source["reason"];
	        this.runId = source["runId"];
	        this.zone = source["zone"];
	    }
	}
	export class ModelView {
	    id: string;
	    displayName?: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.displayName = source["displayName"];
	    }
	}
	export class CatalogView {
	    provider: string;
	    models: ModelView[];
	    kind: string;
	    notice?: string;
	    warning?: string;
	
	    static createFrom(source: any = {}) {
	        return new CatalogView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.models = this.convertValues(source["models"], ModelView);
	        this.kind = source["kind"];
	        this.notice = source["notice"];
	        this.warning = source["warning"];
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
	export class HarnessView {
	    name: string;
	    displayName: string;
	    executable: string;
	    installUrl: string;
	    found: boolean;
	    path?: string;
	    pathFromPolicy: boolean;
	    source?: string;
	    version?: string;
	    versionMatches: boolean;
	    warnings?: string[];
	    reason?: string;
	    chosen: boolean;
	    mcpCommand: string;
	
	    static createFrom(source: any = {}) {
	        return new HarnessView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.displayName = source["displayName"];
	        this.executable = source["executable"];
	        this.installUrl = source["installUrl"];
	        this.found = source["found"];
	        this.path = source["path"];
	        this.pathFromPolicy = source["pathFromPolicy"];
	        this.source = source["source"];
	        this.version = source["version"];
	        this.versionMatches = source["versionMatches"];
	        this.warnings = source["warnings"];
	        this.reason = source["reason"];
	        this.chosen = source["chosen"];
	        this.mcpCommand = source["mcpCommand"];
	    }
	}
	export class CodingAgentsView {
	    chosen: HarnessView[];
	    export: install.CodingAgentExport;
	
	    static createFrom(source: any = {}) {
	        return new CodingAgentsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chosen = this.convertValues(source["chosen"], HarnessView);
	        this.export = this.convertValues(source["export"], install.CodingAgentExport);
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
	export class ConnectView {
	    connection?: run.ServerConnection;
	    needsOwnClient?: run.OwnClientGuide;
	    cancelled?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ConnectView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connection = this.convertValues(source["connection"], run.ServerConnection);
	        this.needsOwnClient = this.convertValues(source["needsOwnClient"], run.OwnClientGuide);
	        this.cancelled = source["cancelled"];
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
	export class CreatorBudgetView {
	    timeoutSeconds: number;
	    maxTurns: number;
	
	    static createFrom(source: any = {}) {
	        return new CreatorBudgetView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.maxTurns = source["maxTurns"];
	    }
	}
	export class CreatorWebView {
	    ready: boolean;
	    why?: string;
	    provider?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreatorWebView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.why = source["why"];
	        this.provider = source["provider"];
	    }
	}
	
	export class DescribeHostedView {
	    offered: boolean;
	    available: boolean;
	    reason?: string;
	    remedy?: string;
	    signedIn: boolean;
	    holdCredits: number;
	    holdSentence?: string;
	    inputsSentence: string;
	    timeoutSeconds: number;
	    maxTurns: number;
	    maxPages: number;
	    knowledgeEnabled: boolean;
	    maxPacks: number;
	    maxBytes: number;
	    knowledgeNote?: string;
	    balance: number;
	    balanceKnown: boolean;
	    pool?: string;
	    unmetered: boolean;
	    short: boolean;
	    reviseOffered: boolean;
	    reviseAvailable: boolean;
	    reviseReason?: string;
	    reviseRemedy?: string;
	    reviseUpgrade: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DescribeHostedView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offered = source["offered"];
	        this.available = source["available"];
	        this.reason = source["reason"];
	        this.remedy = source["remedy"];
	        this.signedIn = source["signedIn"];
	        this.holdCredits = source["holdCredits"];
	        this.holdSentence = source["holdSentence"];
	        this.inputsSentence = source["inputsSentence"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.maxTurns = source["maxTurns"];
	        this.maxPages = source["maxPages"];
	        this.knowledgeEnabled = source["knowledgeEnabled"];
	        this.maxPacks = source["maxPacks"];
	        this.maxBytes = source["maxBytes"];
	        this.knowledgeNote = source["knowledgeNote"];
	        this.balance = source["balance"];
	        this.balanceKnown = source["balanceKnown"];
	        this.pool = source["pool"];
	        this.unmetered = source["unmetered"];
	        this.short = source["short"];
	        this.reviseOffered = source["reviseOffered"];
	        this.reviseAvailable = source["reviseAvailable"];
	        this.reviseReason = source["reviseReason"];
	        this.reviseRemedy = source["reviseRemedy"];
	        this.reviseUpgrade = source["reviseUpgrade"];
	    }
	}
	export class JevKeyView {
	    stored: boolean;
	    source?: string;
	    envName: string;
	    envSet: boolean;
	    ready: boolean;
	    keyStoreErr?: string;
	
	    static createFrom(source: any = {}) {
	        return new JevKeyView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stored = source["stored"];
	        this.source = source["source"];
	        this.envName = source["envName"];
	        this.envSet = source["envSet"];
	        this.ready = source["ready"];
	        this.keyStoreErr = source["keyStoreErr"];
	    }
	}
	export class DescribeStatusView {
	    key: JevKeyView;
	    search: string;
	    searchSentence: string;
	    modelName: string;
	    maxPacks: number;
	    maxBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new DescribeStatusView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = this.convertValues(source["key"], JevKeyView);
	        this.search = source["search"];
	        this.searchSentence = source["searchSentence"];
	        this.modelName = source["modelName"];
	        this.maxPacks = source["maxPacks"];
	        this.maxBytes = source["maxBytes"];
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
	export class DocumentsView {
	    bytes: number;
	    localMaxBytes: number;
	    agentryMaxBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new DocumentsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bytes = source["bytes"];
	        this.localMaxBytes = source["localMaxBytes"];
	        this.agentryMaxBytes = source["agentryMaxBytes"];
	    }
	}
	export class DraftAIReviewView {
	    scan: string;
	    read: number;
	    passed: number;
	    withheld: number;
	    notScanned: number;
	    withheldSources: drafter.WithheldSource[];
	
	    static createFrom(source: any = {}) {
	        return new DraftAIReviewView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scan = source["scan"];
	        this.read = source["read"];
	        this.passed = source["passed"];
	        this.withheld = source["withheld"];
	        this.notScanned = source["notScanned"];
	        this.withheldSources = this.convertValues(source["withheldSources"], drafter.WithheldSource);
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
	export class DraftFindingView {
	    id: string;
	    question: string;
	    score: number;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new DraftFindingView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.question = source["question"];
	        this.score = source["score"];
	        this.path = source["path"];
	    }
	}
	export class DraftCheckView {
	    line?: draft.Line;
	    rubric: string;
	    draft: string;
	    findings: DraftFindingView[];
	    flagged: string[];
	    unanswered: string;
	    checks: number;
	
	    static createFrom(source: any = {}) {
	        return new DraftCheckView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line = this.convertValues(source["line"], draft.Line);
	        this.rubric = source["rubric"];
	        this.draft = source["draft"];
	        this.findings = this.convertValues(source["findings"], DraftFindingView);
	        this.flagged = source["flagged"];
	        this.unanswered = source["unanswered"];
	        this.checks = source["checks"];
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
	
	export class RevisionView {
	    namespace: string;
	    name: string;
	    from: string;
	    version: string;
	    hosted: boolean;
	    key: string;
	    diff: build.DiffFile[];
	
	    static createFrom(source: any = {}) {
	        return new RevisionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.from = source["from"];
	        this.version = source["version"];
	        this.hosted = source["hosted"];
	        this.key = source["key"];
	        this.diff = this.convertValues(source["diff"], build.DiffFile);
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
	export class DraftView {
	    model?: build.Model;
	    decodeNotes: string[];
	    notes: string;
	    matches: drafter.Match[];
	    sources: drafter.Source[];
	    openQuestions: string[];
	    aiReview: DraftAIReviewView;
	    manifestError?: string;
	    drafterModel?: string;
	    runId?: string;
	    outDir: string;
	    where: string;
	    description: string;
	    hints: string[];
	    unmentioned: string[];
	    problems: string[];
	    moreProblems: number;
	    check?: DraftCheckView;
	    knowledge: drafter.PackRead[];
	    revision?: RevisionView;
	
	    static createFrom(source: any = {}) {
	        return new DraftView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = this.convertValues(source["model"], build.Model);
	        this.decodeNotes = source["decodeNotes"];
	        this.notes = source["notes"];
	        this.matches = this.convertValues(source["matches"], drafter.Match);
	        this.sources = this.convertValues(source["sources"], drafter.Source);
	        this.openQuestions = source["openQuestions"];
	        this.aiReview = this.convertValues(source["aiReview"], DraftAIReviewView);
	        this.manifestError = source["manifestError"];
	        this.drafterModel = source["drafterModel"];
	        this.runId = source["runId"];
	        this.outDir = source["outDir"];
	        this.where = source["where"];
	        this.description = source["description"];
	        this.hints = source["hints"];
	        this.unmentioned = source["unmentioned"];
	        this.problems = source["problems"];
	        this.moreProblems = source["moreProblems"];
	        this.check = this.convertValues(source["check"], DraftCheckView);
	        this.knowledge = this.convertValues(source["knowledge"], drafter.PackRead);
	        this.revision = this.convertValues(source["revision"], RevisionView);
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
	export class EditView {
	    model?: build.Model;
	    namespace: string;
	    name: string;
	    from: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new EditView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = this.convertValues(source["model"], build.Model);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.from = source["from"];
	        this.version = source["version"];
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
	export class ExportView {
	    dir: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	    }
	}
	export class FindingView {
	    scanner: string;
	    severity: string;
	    confidence: string;
	    title: string;
	    filePath?: string;
	    lineStart?: number;
	    lineEnd?: number;
	    excerpt?: string;
	    verdict: string;
	    verdictReason?: string;
	
	    static createFrom(source: any = {}) {
	        return new FindingView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scanner = source["scanner"];
	        this.severity = source["severity"];
	        this.confidence = source["confidence"];
	        this.title = source["title"];
	        this.filePath = source["filePath"];
	        this.lineStart = source["lineStart"];
	        this.lineEnd = source["lineEnd"];
	        this.excerpt = source["excerpt"];
	        this.verdict = source["verdict"];
	        this.verdictReason = source["verdictReason"];
	    }
	}
	export class ForkView {
	    cancelled: boolean;
	    namespace?: string;
	    name?: string;
	    sentence?: string;
	    refusal?: string;
	    askName?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ForkView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cancelled = source["cancelled"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.sentence = source["sentence"];
	        this.refusal = source["refusal"];
	        this.askName = source["askName"];
	    }
	}
	export class GuideView {
	    overview: string;
	    prepare: string[];
	    files: string;
	    task: string;
	    connect: {[key: string]: string};
	
	    static createFrom(source: any = {}) {
	        return new GuideView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.overview = source["overview"];
	        this.prepare = source["prepare"];
	        this.files = source["files"];
	        this.task = source["task"];
	        this.connect = source["connect"];
	    }
	}
	
	export class HomeLayoutView {
	    cards: settings.HomeCard[];
	    newer: boolean;
	    unreadable: boolean;
	    maxPins: number;
	    pinsPerCard: number;
	
	    static createFrom(source: any = {}) {
	        return new HomeLayoutView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cards = this.convertValues(source["cards"], settings.HomeCard);
	        this.newer = source["newer"];
	        this.unreadable = source["unreadable"];
	        this.maxPins = source["maxPins"];
	        this.pinsPerCard = source["pinsPerCard"];
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
	export class HostedArtefactView {
	    name: string;
	    bytes: number;
	    path?: string;
	
	    static createFrom(source: any = {}) {
	        return new HostedArtefactView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.bytes = source["bytes"];
	        this.path = source["path"];
	    }
	}
	export class HostedScheduleOfferView {
	    offered: boolean;
	    available: boolean;
	    reason?: string;
	    remedy?: string;
	    upgrade?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HostedScheduleOfferView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offered = source["offered"];
	        this.available = source["available"];
	        this.reason = source["reason"];
	        this.remedy = source["remedy"];
	        this.upgrade = source["upgrade"];
	    }
	}
	export class HostedSessionOfferView {
	    offered: boolean;
	    available: boolean;
	    reason?: string;
	    remedy?: string;
	    upgrade?: boolean;
	    freeMinutes: number;
	    creditsPerMinute: number;
	    minCredits: number;
	    idleSeconds: number;
	    transport: string;
	
	    static createFrom(source: any = {}) {
	        return new HostedSessionOfferView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offered = source["offered"];
	        this.available = source["available"];
	        this.reason = source["reason"];
	        this.remedy = source["remedy"];
	        this.upgrade = source["upgrade"];
	        this.freeMinutes = source["freeMinutes"];
	        this.creditsPerMinute = source["creditsPerMinute"];
	        this.minCredits = source["minCredits"];
	        this.idleSeconds = source["idleSeconds"];
	        this.transport = source["transport"];
	    }
	}
	export class HostedCapsView {
	    timeoutSeconds: number;
	    maxTurns: number;
	
	    static createFrom(source: any = {}) {
	        return new HostedCapsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.maxTurns = source["maxTurns"];
	    }
	}
	export class HostedAvailabilityView {
	    offered: boolean;
	    available: boolean;
	    reason?: string;
	    remedy?: string;
	    signedIn: boolean;
	    extension: boolean;
	    enabled: boolean;
	    runnersOnline: number;
	    queued: number;
	    tier?: string;
	    holdCredits: number;
	    caps?: HostedCapsView;
	    balance: number;
	    balanceKnown: boolean;
	    knowledgeAttach: boolean;
	    maxPacks?: number;
	    knowledgeNote?: string;
	    maxInputFiles: number;
	    maxInputBytes: number;
	    declaredSecrets?: string[];
	    optionalSecrets?: string[];
	    vault: boolean;
	    session?: HostedSessionOfferView;
	    schedule?: HostedScheduleOfferView;
	    publisherEarns: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HostedAvailabilityView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offered = source["offered"];
	        this.available = source["available"];
	        this.reason = source["reason"];
	        this.remedy = source["remedy"];
	        this.signedIn = source["signedIn"];
	        this.extension = source["extension"];
	        this.enabled = source["enabled"];
	        this.runnersOnline = source["runnersOnline"];
	        this.queued = source["queued"];
	        this.tier = source["tier"];
	        this.holdCredits = source["holdCredits"];
	        this.caps = this.convertValues(source["caps"], HostedCapsView);
	        this.balance = source["balance"];
	        this.balanceKnown = source["balanceKnown"];
	        this.knowledgeAttach = source["knowledgeAttach"];
	        this.maxPacks = source["maxPacks"];
	        this.knowledgeNote = source["knowledgeNote"];
	        this.maxInputFiles = source["maxInputFiles"];
	        this.maxInputBytes = source["maxInputBytes"];
	        this.declaredSecrets = source["declaredSecrets"];
	        this.optionalSecrets = source["optionalSecrets"];
	        this.vault = source["vault"];
	        this.session = this.convertValues(source["session"], HostedSessionOfferView);
	        this.schedule = this.convertValues(source["schedule"], HostedScheduleOfferView);
	        this.publisherEarns = source["publisherEarns"];
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
	export class HostedCalendarView {
	    fires: CalendarFire[];
	    unavailable?: string;
	
	    static createFrom(source: any = {}) {
	        return new HostedCalendarView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fires = this.convertValues(source["fires"], CalendarFire);
	        this.unavailable = source["unavailable"];
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
	
	export class HostedDraftView {
	    id: string;
	    status: string;
	    state: string;
	    statusLine: string;
	    queuePosition?: number;
	    cancelRequested: boolean;
	    outcome?: string;
	    outcomeLine?: string;
	    outcomeTone?: string;
	    error?: string;
	    holdCredits: number;
	    creditsLine: string;
	    draft?: DraftView;
	    draftError?: string;
	    revises?: string;
	    final: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HostedDraftView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.status = source["status"];
	        this.state = source["state"];
	        this.statusLine = source["statusLine"];
	        this.queuePosition = source["queuePosition"];
	        this.cancelRequested = source["cancelRequested"];
	        this.outcome = source["outcome"];
	        this.outcomeLine = source["outcomeLine"];
	        this.outcomeTone = source["outcomeTone"];
	        this.error = source["error"];
	        this.holdCredits = source["holdCredits"];
	        this.creditsLine = source["creditsLine"];
	        this.draft = this.convertValues(source["draft"], DraftView);
	        this.draftError = source["draftError"];
	        this.revises = source["revises"];
	        this.final = source["final"];
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
	export class HostedDraftStartView {
	    draft?: HostedDraftView;
	    refusal?: string;
	    detail?: string;
	    balance?: number;
	    hold?: number;
	    resetsAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new HostedDraftStartView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.draft = this.convertValues(source["draft"], HostedDraftView);
	        this.refusal = source["refusal"];
	        this.detail = source["detail"];
	        this.balance = source["balance"];
	        this.hold = source["hold"];
	        this.resetsAt = source["resetsAt"];
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
	
	export class HostedPackView {
	    name: string;
	    digest?: string;
	
	    static createFrom(source: any = {}) {
	        return new HostedPackView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.digest = source["digest"];
	    }
	}
	export class HostedSessionView {
	    freeMinutes: number;
	    creditsPerMinute: number;
	    paidThrough: number;
	    timeCredits: number;
	    tokenCredits: number;
	    tokensUnpaid: number;
	    outOfCredits: boolean;
	    filesAdded: number;
	    idleSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new HostedSessionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.freeMinutes = source["freeMinutes"];
	        this.creditsPerMinute = source["creditsPerMinute"];
	        this.paidThrough = source["paidThrough"];
	        this.timeCredits = source["timeCredits"];
	        this.tokenCredits = source["tokenCredits"];
	        this.tokensUnpaid = source["tokensUnpaid"];
	        this.outOfCredits = source["outOfCredits"];
	        this.filesAdded = source["filesAdded"];
	        this.idleSeconds = source["idleSeconds"];
	    }
	}
	export class HostedRunView {
	    id: string;
	    ref: string;
	    digest: string;
	    status: string;
	    state: string;
	    statusLine: string;
	    queuePosition?: number;
	    cancelRequested: boolean;
	    outcome?: string;
	    exitClass?: string;
	    outcomeLine?: string;
	    outcomeTone?: string;
	    error?: string;
	    turns: number;
	    wallMs: number;
	    holdCredits: number;
	    settledCredits: number;
	    settled: boolean;
	    receiptKept: boolean;
	    capabilitiesEnforced?: {[key: string]: string};
	    knowledgeLine?: string;
	    knowledge?: HostedPackView[];
	    attestationState?: string;
	    adapter?: string;
	    resultJson?: string;
	    outDir?: string;
	    outputs: HostedArtefactView[];
	    downloadError?: string;
	    caps?: HostedCapsView;
	    mode?: string;
	    task?: string;
	    session?: HostedSessionView;
	    startedAt?: string;
	    createdAt?: string;
	    finishedAt?: string;
	    final: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HostedRunView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ref = source["ref"];
	        this.digest = source["digest"];
	        this.status = source["status"];
	        this.state = source["state"];
	        this.statusLine = source["statusLine"];
	        this.queuePosition = source["queuePosition"];
	        this.cancelRequested = source["cancelRequested"];
	        this.outcome = source["outcome"];
	        this.exitClass = source["exitClass"];
	        this.outcomeLine = source["outcomeLine"];
	        this.outcomeTone = source["outcomeTone"];
	        this.error = source["error"];
	        this.turns = source["turns"];
	        this.wallMs = source["wallMs"];
	        this.holdCredits = source["holdCredits"];
	        this.settledCredits = source["settledCredits"];
	        this.settled = source["settled"];
	        this.receiptKept = source["receiptKept"];
	        this.capabilitiesEnforced = source["capabilitiesEnforced"];
	        this.knowledgeLine = source["knowledgeLine"];
	        this.knowledge = this.convertValues(source["knowledge"], HostedPackView);
	        this.attestationState = source["attestationState"];
	        this.adapter = source["adapter"];
	        this.resultJson = source["resultJson"];
	        this.outDir = source["outDir"];
	        this.outputs = this.convertValues(source["outputs"], HostedArtefactView);
	        this.downloadError = source["downloadError"];
	        this.caps = this.convertValues(source["caps"], HostedCapsView);
	        this.mode = source["mode"];
	        this.task = source["task"];
	        this.session = this.convertValues(source["session"], HostedSessionView);
	        this.startedAt = source["startedAt"];
	        this.createdAt = source["createdAt"];
	        this.finishedAt = source["finishedAt"];
	        this.final = source["final"];
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
	export class HostedRecordView {
	    run?: HostedRunView;
	    kind: string;
	    kept: boolean;
	    folderState: string;
	    folderError?: string;
	    downloadable: boolean;
	    followed: boolean;
	    noOutput?: string;
	
	    static createFrom(source: any = {}) {
	        return new HostedRecordView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run = this.convertValues(source["run"], HostedRunView);
	        this.kind = source["kind"];
	        this.kept = source["kept"];
	        this.folderState = source["folderState"];
	        this.folderError = source["folderError"];
	        this.downloadable = source["downloadable"];
	        this.followed = source["followed"];
	        this.noOutput = source["noOutput"];
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
	export class HostedRunNowView {
	    runId?: string;
	    refusal?: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new HostedRunNowView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.refusal = source["refusal"];
	        this.detail = source["detail"];
	    }
	}
	
	export class HostedRunsTermsView {
	    holdCredits: number;
	    caps: HostedCapsView;
	
	    static createFrom(source: any = {}) {
	        return new HostedRunsTermsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.holdCredits = source["holdCredits"];
	        this.caps = this.convertValues(source["caps"], HostedCapsView);
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
	export class HostedScheduleItem {
	    hosted: boolean;
	    registry: string;
	    id: string;
	    name: string;
	    ref: string;
	    version: string;
	    describe: string;
	    when: schedules.When;
	    paused?: boolean;
	    pausedReason?: string;
	    ended?: boolean;
	    runs?: number;
	    refusals?: number;
	    next?: string;
	    files?: string[];
	
	    static createFrom(source: any = {}) {
	        return new HostedScheduleItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hosted = source["hosted"];
	        this.registry = source["registry"];
	        this.id = source["id"];
	        this.name = source["name"];
	        this.ref = source["ref"];
	        this.version = source["version"];
	        this.describe = source["describe"];
	        this.when = this.convertValues(source["when"], schedules.When);
	        this.paused = source["paused"];
	        this.pausedReason = source["pausedReason"];
	        this.ended = source["ended"];
	        this.runs = source["runs"];
	        this.refusals = source["refusals"];
	        this.next = source["next"];
	        this.files = source["files"];
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
	export class HostedScheduleMadeView {
	    id?: string;
	    next?: string;
	    refusal?: string;
	    detail?: string;
	    missing?: string[];
	    optional?: string[];
	
	    static createFrom(source: any = {}) {
	        return new HostedScheduleMadeView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.next = source["next"];
	        this.refusal = source["refusal"];
	        this.detail = source["detail"];
	        this.missing = source["missing"];
	        this.optional = source["optional"];
	    }
	}
	
	export class HostedScheduleRequest {
	    registry: string;
	    ref: string;
	    name?: string;
	    task?: string;
	    files?: string[];
	    knowledge?: string[];
	    intake?: {[key: string]: string};
	    voice: boolean;
	    when: schedules.When;
	    askOptionalSecrets?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HostedScheduleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registry = source["registry"];
	        this.ref = source["ref"];
	        this.name = source["name"];
	        this.task = source["task"];
	        this.files = source["files"];
	        this.knowledge = source["knowledge"];
	        this.intake = source["intake"];
	        this.voice = source["voice"];
	        this.when = this.convertValues(source["when"], schedules.When);
	        this.askOptionalSecrets = source["askOptionalSecrets"];
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
	export class HostedSchedulesView {
	    registry: string;
	    signedOut?: boolean;
	    unavailable?: string;
	    schedules: HostedScheduleItem[];
	    limit?: number;
	
	    static createFrom(source: any = {}) {
	        return new HostedSchedulesView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registry = source["registry"];
	        this.signedOut = source["signedOut"];
	        this.unavailable = source["unavailable"];
	        this.schedules = this.convertValues(source["schedules"], HostedScheduleItem);
	        this.limit = source["limit"];
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
	
	
	export class HostedStartView {
	    run?: HostedRunView;
	    refusal?: string;
	    detail?: string;
	    balance?: number;
	    hold?: number;
	    resetsAt?: string;
	    missing?: string[];
	    optional?: string[];
	
	    static createFrom(source: any = {}) {
	        return new HostedStartView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run = this.convertValues(source["run"], HostedRunView);
	        this.refusal = source["refusal"];
	        this.detail = source["detail"];
	        this.balance = source["balance"];
	        this.hold = source["hold"];
	        this.resetsAt = source["resetsAt"];
	        this.missing = source["missing"];
	        this.optional = source["optional"];
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
	export class HybridKeyTestView {
	    ok: boolean;
	    message: string;
	    from?: string;
	
	    static createFrom(source: any = {}) {
	        return new HybridKeyTestView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	        this.from = source["from"];
	    }
	}
	export class HybridKeyView {
	    stored: boolean;
	    source?: string;
	    envName: string;
	    envSet: boolean;
	    ready: boolean;
	    keyStoreErr?: string;
	
	    static createFrom(source: any = {}) {
	        return new HybridKeyView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stored = source["stored"];
	        this.source = source["source"];
	        this.envName = source["envName"];
	        this.envSet = source["envSet"];
	        this.ready = source["ready"];
	        this.keyStoreErr = source["keyStoreErr"];
	    }
	}
	export class ImageView {
	    ref: string;
	    size?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImageView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.size = source["size"];
	    }
	}
	export class ImagesView {
	    images: ImageView[];
	    unavailable?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImagesView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.images = this.convertValues(source["images"], ImageView);
	        this.unavailable = source["unavailable"];
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
	export class InFlightView {
	    kind: string;
	    line: string;
	
	    static createFrom(source: any = {}) {
	        return new InFlightView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.line = source["line"];
	    }
	}
	export class InstallUpdatesView {
	    checked?: string;
	    available: number;
	    updates: install.UpdateInfo[];
	    err?: string;
	
	    static createFrom(source: any = {}) {
	        return new InstallUpdatesView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.checked = source["checked"];
	        this.available = source["available"];
	        this.updates = this.convertValues(source["updates"], install.UpdateInfo);
	        this.err = source["err"];
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
	export class IntakeQuestionView {
	    id: string;
	    prompt: string;
	    type: string;
	    required: boolean;
	    choices: string[];
	
	    static createFrom(source: any = {}) {
	        return new IntakeQuestionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.prompt = source["prompt"];
	        this.type = source["type"];
	        this.required = source["required"];
	        this.choices = source["choices"];
	    }
	}
	export class JevKeyTestView {
	    ok: boolean;
	    message: string;
	    from?: string;
	
	    static createFrom(source: any = {}) {
	        return new JevKeyTestView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	        this.from = source["from"];
	    }
	}
	
	export class KeptConnectionsView {
	    connections: run.KeptConnection[];
	    clients: run.KeptClient[];
	
	    static createFrom(source: any = {}) {
	        return new KeptConnectionsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connections = this.convertValues(source["connections"], run.KeptConnection);
	        this.clients = this.convertValues(source["clients"], run.KeptClient);
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
	export class KnowledgeAcceptsView {
	    prompt: string;
	    required: boolean;
	
	    static createFrom(source: any = {}) {
	        return new KnowledgeAcceptsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.prompt = source["prompt"];
	        this.required = source["required"];
	    }
	}
	export class KnowledgeHostedAvailabilityView {
	    offered: boolean;
	    available: boolean;
	    reason?: string;
	    remedy?: string;
	    upgradeUrl?: string;
	    holdCredits: number;
	    scanCredits: number;
	    timeoutSeconds: number;
	    maxTurns: number;
	    balance: number;
	    balanceKnown: boolean;
	    pool?: string;
	    unmetered: boolean;
	    maxInputFiles: number;
	    maxInputBytes: number;
	    takesBrief: boolean;
	    maxBriefBytes: number;
	    web: boolean;
	    webNote?: string;
	
	    static createFrom(source: any = {}) {
	        return new KnowledgeHostedAvailabilityView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offered = source["offered"];
	        this.available = source["available"];
	        this.reason = source["reason"];
	        this.remedy = source["remedy"];
	        this.upgradeUrl = source["upgradeUrl"];
	        this.holdCredits = source["holdCredits"];
	        this.scanCredits = source["scanCredits"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.maxTurns = source["maxTurns"];
	        this.balance = source["balance"];
	        this.balanceKnown = source["balanceKnown"];
	        this.pool = source["pool"];
	        this.unmetered = source["unmetered"];
	        this.maxInputFiles = source["maxInputFiles"];
	        this.maxInputBytes = source["maxInputBytes"];
	        this.takesBrief = source["takesBrief"];
	        this.maxBriefBytes = source["maxBriefBytes"];
	        this.web = source["web"];
	        this.webNote = source["webNote"];
	    }
	}
	export class KnowledgeHostedBuildView {
	    id: string;
	    name: string;
	    status: string;
	    state: string;
	    statusLine: string;
	    queuePosition?: number;
	    cancelRequested: boolean;
	    outcome?: string;
	    outcomeLine?: string;
	    outcomeTone?: string;
	    error?: string;
	    holdCredits: number;
	    settledCredits: number;
	    settled: boolean;
	    scanCredits: number;
	    applied: boolean;
	    pages: number;
	    bytes: number;
	    note?: string;
	    turns: number;
	    documents: HostedArtefactView[];
	    brief?: string;
	    webLine?: string;
	    canContinue: boolean;
	    downloaded: boolean;
	    kept?: string;
	    downloadError?: string;
	    scanState?: string;
	    scan?: string;
	    final: boolean;
	
	    static createFrom(source: any = {}) {
	        return new KnowledgeHostedBuildView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.state = source["state"];
	        this.statusLine = source["statusLine"];
	        this.queuePosition = source["queuePosition"];
	        this.cancelRequested = source["cancelRequested"];
	        this.outcome = source["outcome"];
	        this.outcomeLine = source["outcomeLine"];
	        this.outcomeTone = source["outcomeTone"];
	        this.error = source["error"];
	        this.holdCredits = source["holdCredits"];
	        this.settledCredits = source["settledCredits"];
	        this.settled = source["settled"];
	        this.scanCredits = source["scanCredits"];
	        this.applied = source["applied"];
	        this.pages = source["pages"];
	        this.bytes = source["bytes"];
	        this.note = source["note"];
	        this.turns = source["turns"];
	        this.documents = this.convertValues(source["documents"], HostedArtefactView);
	        this.brief = source["brief"];
	        this.webLine = source["webLine"];
	        this.canContinue = source["canContinue"];
	        this.downloaded = source["downloaded"];
	        this.kept = source["kept"];
	        this.downloadError = source["downloadError"];
	        this.scanState = source["scanState"];
	        this.scan = source["scan"];
	        this.final = source["final"];
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
	export class KnowledgeHostedStartView {
	    build?: KnowledgeHostedBuildView;
	    refusal?: string;
	    detail?: string;
	    balance?: number;
	    hold?: number;
	    resetsAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new KnowledgeHostedStartView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.build = this.convertValues(source["build"], KnowledgeHostedBuildView);
	        this.refusal = source["refusal"];
	        this.detail = source["detail"];
	        this.balance = source["balance"];
	        this.hold = source["hold"];
	        this.resetsAt = source["resetsAt"];
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
	export class LocalInstallView {
	    ref: string;
	    digest: string;
	    repaired: boolean;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new LocalInstallView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.digest = source["digest"];
	        this.repaired = source["repaired"];
	        this.warnings = source["warnings"];
	    }
	}
	export class MCPToolRow {
	    tool: string;
	    grantedBy: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPToolRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tool = source["tool"];
	        this.grantedBy = source["grantedBy"];
	    }
	}
	export class MCPView {
	    ref: string;
	    version: string;
	    toolRows: MCPToolRow[];
	    servers: string[];
	    declaredServers: mcpview.Server[];
	    secretNames: string[];
	    dataAccess: string[];
	    embeddedTools: string[];
	    capabilityLines: string[];
	    vulnerabilities: mcpvuln.Status[];
	    vulnerabilityLine?: string;
	    refusal?: string;
	    refusing?: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.version = source["version"];
	        this.toolRows = this.convertValues(source["toolRows"], MCPToolRow);
	        this.servers = source["servers"];
	        this.declaredServers = this.convertValues(source["declaredServers"], mcpview.Server);
	        this.secretNames = source["secretNames"];
	        this.dataAccess = source["dataAccess"];
	        this.embeddedTools = source["embeddedTools"];
	        this.capabilityLines = source["capabilityLines"];
	        this.vulnerabilities = this.convertValues(source["vulnerabilities"], mcpvuln.Status);
	        this.vulnerabilityLine = source["vulnerabilityLine"];
	        this.refusal = source["refusal"];
	        this.refusing = source["refusing"];
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
	export class MetaView {
	    version: string;
	    apiVersion: string;
	    minCliVersion?: string;
	    features: {[key: string]: boolean};
	
	    static createFrom(source: any = {}) {
	        return new MetaView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.apiVersion = source["apiVersion"];
	        this.minCliVersion = source["minCliVersion"];
	        this.features = source["features"];
	    }
	}
	
	export class OrgMembership {
	    handle: string;
	    role: string;
	    state?: string;
	
	    static createFrom(source: any = {}) {
	        return new OrgMembership(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.handle = source["handle"];
	        this.role = source["role"];
	        this.state = source["state"];
	    }
	}
	export class OutputPDFView {
	    exported: boolean;
	    pages?: number;
	    theme?: string;
	
	    static createFrom(source: any = {}) {
	        return new OutputPDFView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exported = source["exported"];
	        this.pages = source["pages"];
	        this.theme = source["theme"];
	    }
	}
	export class OutputFileView {
	    path: string;
	    bytes: number;
	    previewKind: string;
	    preview?: string;
	    noPreview?: string;
	    pdf?: OutputPDFView;
	
	    static createFrom(source: any = {}) {
	        return new OutputFileView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.bytes = source["bytes"];
	        this.previewKind = source["previewKind"];
	        this.preview = source["preview"];
	        this.noPreview = source["noPreview"];
	        this.pdf = this.convertValues(source["pdf"], OutputPDFView);
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
	
	export class PendingOrgView {
	    handle: string;
	    displayName: string;
	    via: string;
	
	    static createFrom(source: any = {}) {
	        return new PendingOrgView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.handle = source["handle"];
	        this.displayName = source["displayName"];
	        this.via = source["via"];
	    }
	}
	export class PrereqView {
	    dockerOk: boolean;
	    dockerErr?: string;
	    os: string;
	    distro?: string;
	    canInstall: boolean;
	    command?: string;
	    downloadUrl: string;
	    note?: string;
	    state: string;
	    engineInstalled: boolean;
	    inDockerGroup: boolean;
	    groupCommand?: string;
	
	    static createFrom(source: any = {}) {
	        return new PrereqView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dockerOk = source["dockerOk"];
	        this.dockerErr = source["dockerErr"];
	        this.os = source["os"];
	        this.distro = source["distro"];
	        this.canInstall = source["canInstall"];
	        this.command = source["command"];
	        this.downloadUrl = source["downloadUrl"];
	        this.note = source["note"];
	        this.state = source["state"];
	        this.engineInstalled = source["engineInstalled"];
	        this.inDockerGroup = source["inDockerGroup"];
	        this.groupCommand = source["groupCommand"];
	    }
	}
	export class ProfileStatusView {
	    ref: string;
	    digest: string;
	    declaredSecrets: string[];
	    optionalSecrets: string[];
	    exists: boolean;
	    keep: boolean;
	    storedSecretNames: string[];
	    capabilityLines: string[];
	    intake: IntakeQuestionView[];
	    modes: string[];
	    opening: string;
	    guide?: GuideView;
	    shipsKnowledge: string[];
	    acceptsKnowledge?: KnowledgeAcceptsView;
	
	    static createFrom(source: any = {}) {
	        return new ProfileStatusView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.digest = source["digest"];
	        this.declaredSecrets = source["declaredSecrets"];
	        this.optionalSecrets = source["optionalSecrets"];
	        this.exists = source["exists"];
	        this.keep = source["keep"];
	        this.storedSecretNames = source["storedSecretNames"];
	        this.capabilityLines = source["capabilityLines"];
	        this.intake = this.convertValues(source["intake"], IntakeQuestionView);
	        this.modes = source["modes"];
	        this.opening = source["opening"];
	        this.guide = this.convertValues(source["guide"], GuideView);
	        this.shipsKnowledge = source["shipsKnowledge"];
	        this.acceptsKnowledge = this.convertValues(source["acceptsKnowledge"], KnowledgeAcceptsView);
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
	export class SuspendedView {
	    since: string;
	    owed: number;
	
	    static createFrom(source: any = {}) {
	        return new SuspendedView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.since = source["since"];
	        this.owed = source["owed"];
	    }
	}
	export class TopUpPack {
	    id: string;
	    credits: number;
	    baseCredits: number;
	    amount: number;
	    currency: string;
	
	    static createFrom(source: any = {}) {
	        return new TopUpPack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.credits = source["credits"];
	        this.baseCredits = source["baseCredits"];
	        this.amount = source["amount"];
	        this.currency = source["currency"];
	    }
	}
	export class TopUpView {
	    available: boolean;
	    url?: string;
	    processing: boolean;
	    packs: TopUpPack[];
	
	    static createFrom(source: any = {}) {
	        return new TopUpView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.url = source["url"];
	        this.processing = source["processing"];
	        this.packs = this.convertValues(source["packs"], TopUpPack);
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
	export class UpgradeRef {
	    available: boolean;
	    manage: boolean;
	    url?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpgradeRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.manage = source["manage"];
	        this.url = source["url"];
	    }
	}
	export class PromoView {
	    state: string;
	    code?: string;
	    credits?: number;
	    creditsUsed?: number;
	    endsAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new PromoView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.code = source["code"];
	        this.credits = source["credits"];
	        this.creditsUsed = source["creditsUsed"];
	        this.endsAt = source["endsAt"];
	    }
	}
	export class KnowledgeAllocation {
	    enabled: boolean;
	    bytes: number;
	    max: number;
	
	    static createFrom(source: any = {}) {
	        return new KnowledgeAllocation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.bytes = source["bytes"];
	        this.max = source["max"];
	    }
	}
	export class Seats {
	    used: number;
	    max: number;
	
	    static createFrom(source: any = {}) {
	        return new Seats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.used = source["used"];
	        this.max = source["max"];
	    }
	}
	export class SubscriptionView {
	    tier: string;
	    standing?: string;
	    unmetered?: boolean;
	    // Go type: Seats
	    seats?: any;
	    memberOf?: string;
	    balance: number;
	    allowance: number;
	    submissionCost: number;
	    periodEnd: string;
	    pendingTier?: string;
	    tierSince?: string;
	    // Go type: struct { Max int64 "json:\"max\""; Used int64 "json:\"used\""; Enabled *bool "json:\"enabled,omitempty\"" }
	    privateAgents: any;
	    // Go type: KnowledgeAllocation
	    knowledge?: any;
	    // Go type: PromoView
	    promo?: any;
	    // Go type: UpgradeRef
	    upgrade: any;
	    bought?: number;
	    // Go type: TopUpView
	    topUp?: any;
	    // Go type: SuspendedView
	    suspended?: any;
	
	    static createFrom(source: any = {}) {
	        return new SubscriptionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tier = source["tier"];
	        this.standing = source["standing"];
	        this.unmetered = source["unmetered"];
	        this.seats = this.convertValues(source["seats"], null);
	        this.memberOf = source["memberOf"];
	        this.balance = source["balance"];
	        this.allowance = source["allowance"];
	        this.submissionCost = source["submissionCost"];
	        this.periodEnd = source["periodEnd"];
	        this.pendingTier = source["pendingTier"];
	        this.tierSince = source["tierSince"];
	        this.privateAgents = this.convertValues(source["privateAgents"], Object);
	        this.knowledge = this.convertValues(source["knowledge"], null);
	        this.promo = this.convertValues(source["promo"], null);
	        this.upgrade = this.convertValues(source["upgrade"], null);
	        this.bought = source["bought"];
	        this.topUp = this.convertValues(source["topUp"], null);
	        this.suspended = this.convertValues(source["suspended"], null);
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
	export class UserView {
	    handle: string;
	    displayName: string;
	    email: string;
	    emailVerified: boolean;
	    orgs?: OrgMembership[];
	    pendingOrg?: PendingOrgView;
	
	    static createFrom(source: any = {}) {
	        return new UserView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.handle = source["handle"];
	        this.displayName = source["displayName"];
	        this.email = source["email"];
	        this.emailVerified = source["emailVerified"];
	        this.orgs = this.convertValues(source["orgs"], OrgMembership);
	        this.pendingOrg = this.convertValues(source["pendingOrg"], PendingOrgView);
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
	export class ProfileView {
	    user: UserView;
	    // Go type: SubscriptionView
	    subscription?: any;
	    subscriptionError?: string;
	    signedUpWith?: string;
	    hostedRuns?: HostedRunsTermsView;
	
	    static createFrom(source: any = {}) {
	        return new ProfileView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user = this.convertValues(source["user"], UserView);
	        this.subscription = this.convertValues(source["subscription"], null);
	        this.subscriptionError = source["subscriptionError"];
	        this.signedUpWith = source["signedUpWith"];
	        this.hostedRuns = this.convertValues(source["hostedRuns"], HostedRunsTermsView);
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
	export class PublisherForkOrigin {
	    namespace: string;
	    name: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new PublisherForkOrigin(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.version = source["version"];
	    }
	}
	export class PublisherAgent {
	    namespace: string;
	    name: string;
	    summary: string;
	    visibility: string;
	    retiredAt?: string;
	    quarantineState: string;
	    forkedFrom?: PublisherForkOrigin;
	    forkState?: string;
	    installs30d?: number;
	    lastSubmittedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new PublisherAgent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.summary = source["summary"];
	        this.visibility = source["visibility"];
	        this.retiredAt = source["retiredAt"];
	        this.quarantineState = source["quarantineState"];
	        this.forkedFrom = this.convertValues(source["forkedFrom"], PublisherForkOrigin);
	        this.forkState = source["forkState"];
	        this.installs30d = source["installs30d"];
	        this.lastSubmittedAt = source["lastSubmittedAt"];
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
	export class PublisherDone {
	    cancelled: boolean;
	    sentence?: string;
	    agent?: PublisherAgent;
	
	    static createFrom(source: any = {}) {
	        return new PublisherDone(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cancelled = source["cancelled"];
	        this.sentence = source["sentence"];
	        this.agent = this.convertValues(source["agent"], PublisherAgent);
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
	
	export class PublisherNamespace {
	    handle: string;
	    kind: string;
	    admin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PublisherNamespace(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.handle = source["handle"];
	        this.kind = source["kind"];
	        this.admin = source["admin"];
	    }
	}
	export class PublisherQuery {
	    q: string;
	    state: string;
	    forks: string;
	    sort: string;
	
	    static createFrom(source: any = {}) {
	        return new PublisherQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.q = source["q"];
	        this.state = source["state"];
	        this.forks = source["forks"];
	        this.sort = source["sort"];
	    }
	}
	export class PublisherView {
	    signedIn: boolean;
	    namespaces: PublisherNamespace[];
	    canManage: boolean;
	    scopesKnown: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PublisherView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.signedIn = source["signedIn"];
	        this.namespaces = this.convertValues(source["namespaces"], PublisherNamespace);
	        this.canManage = source["canManage"];
	        this.scopesKnown = source["scopesKnown"];
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
	export class RecentRunView {
	    id: string;
	    kind: string;
	    where: string;
	    registry?: string;
	    ref?: string;
	    startedAt?: string;
	    endedAt?: string;
	    outcome: string;
	    exitCode?: number;
	    exitClass?: string;
	    state?: string;
	    tone: string;
	    kept: boolean;
	    source: string;
	    scheduled?: boolean;
	    schedule?: string;
	
	    static createFrom(source: any = {}) {
	        return new RecentRunView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.where = source["where"];
	        this.registry = source["registry"];
	        this.ref = source["ref"];
	        this.startedAt = source["startedAt"];
	        this.endedAt = source["endedAt"];
	        this.outcome = source["outcome"];
	        this.exitCode = source["exitCode"];
	        this.exitClass = source["exitClass"];
	        this.state = source["state"];
	        this.tone = source["tone"];
	        this.kept = source["kept"];
	        this.source = source["source"];
	        this.scheduled = source["scheduled"];
	        this.schedule = source["schedule"];
	    }
	}
	export class RecentRunsView {
	    runs: RecentRunView[];
	    remember: boolean;
	    signedIn: boolean;
	    hostedError?: string;
	    historyError?: string;
	
	    static createFrom(source: any = {}) {
	        return new RecentRunsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runs = this.convertValues(source["runs"], RecentRunView);
	        this.remember = source["remember"];
	        this.signedIn = source["signedIn"];
	        this.hostedError = source["hostedError"];
	        this.historyError = source["historyError"];
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
	export class RecordedRunView {
	    run: RecentRunView;
	    folder?: string;
	    folderState: string;
	    folderError?: string;
	    resultJson?: string;
	
	    static createFrom(source: any = {}) {
	        return new RecordedRunView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run = this.convertValues(source["run"], RecentRunView);
	        this.folder = source["folder"];
	        this.folderState = source["folderState"];
	        this.folderError = source["folderError"];
	        this.resultJson = source["resultJson"];
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
	export class RegistryView {
	    url: string;
	    hasToken: boolean;
	    tokenSource?: string;
	    isDefault: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RegistryView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.hasToken = source["hasToken"];
	        this.tokenSource = source["tokenSource"];
	        this.isDefault = source["isDefault"];
	    }
	}
	export class ReportAccentCheck {
	    valid: boolean;
	    ok: boolean;
	    ratio: number;
	    need: number;
	    onText: boolean;
	    nearest: string;
	
	    static createFrom(source: any = {}) {
	        return new ReportAccentCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.valid = source["valid"];
	        this.ok = source["ok"];
	        this.ratio = source["ratio"];
	        this.need = source["need"];
	        this.onText = source["onText"];
	        this.nearest = source["nearest"];
	    }
	}
	export class ReportDesignProblem {
	    field: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ReportDesignProblem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.field = source["field"];
	        this.message = source["message"];
	    }
	}
	export class ReportDesignCheck {
	    problem?: ReportDesignProblem;
	    accent: ReportAccentCheck;
	    tint: string;
	    unedited: boolean;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new ReportDesignCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.problem = this.convertValues(source["problem"], ReportDesignProblem);
	        this.accent = this.convertValues(source["accent"], ReportAccentCheck);
	        this.tint = source["tint"];
	        this.unedited = source["unedited"];
	        this.label = source["label"];
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
	export class ReportDesignFamily {
	    name: string;
	    kind: string;
	    headingsOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ReportDesignFamily(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.headingsOnly = source["headingsOnly"];
	    }
	}
	export class ReportDesignTheme {
	    id: string;
	    design: reportdesign.Design;
	    thumbnail: string;
	
	    static createFrom(source: any = {}) {
	        return new ReportDesignTheme(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.design = this.convertValues(source["design"], reportdesign.Design);
	        this.thumbnail = source["thumbnail"];
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
	export class ReportDesignOptions {
	    themes: ReportDesignTheme[];
	    families: ReportDesignFamily[];
	    headingFonts: string[];
	    bodyFonts: string[];
	    codeFonts: string[];
	    papers: string[];
	    titleBlocks: string[];
	    headingStyles: string[];
	    quoteStyles: string[];
	    pageNumbers: string[];
	    maxName: number;
	    maxFooterText: number;
	    defaultThemeId: string;
	
	    static createFrom(source: any = {}) {
	        return new ReportDesignOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.themes = this.convertValues(source["themes"], ReportDesignTheme);
	        this.families = this.convertValues(source["families"], ReportDesignFamily);
	        this.headingFonts = source["headingFonts"];
	        this.bodyFonts = source["bodyFonts"];
	        this.codeFonts = source["codeFonts"];
	        this.papers = source["papers"];
	        this.titleBlocks = source["titleBlocks"];
	        this.headingStyles = source["headingStyles"];
	        this.quoteStyles = source["quoteStyles"];
	        this.pageNumbers = source["pageNumbers"];
	        this.maxName = source["maxName"];
	        this.maxFooterText = source["maxFooterText"];
	        this.defaultThemeId = source["defaultThemeId"];
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
	
	
	export class ReportDesignView {
	    available: boolean;
	    design?: reportdesign.Design;
	    label?: string;
	    unreadable?: string;
	    logo?: string;
	
	    static createFrom(source: any = {}) {
	        return new ReportDesignView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.design = this.convertValues(source["design"], reportdesign.Design);
	        this.label = source["label"];
	        this.unreadable = source["unreadable"];
	        this.logo = source["logo"];
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
	export class ReportView {
	    versionStatus: string;
	    scanStatus?: string;
	    verdict?: string;
	    rulesetVersion?: string;
	    judgeModel?: string;
	    finishedAt?: string;
	    findingCounts?: {[key: string]: number};
	    findings: FindingView[];
	
	    static createFrom(source: any = {}) {
	        return new ReportView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.versionStatus = source["versionStatus"];
	        this.scanStatus = source["scanStatus"];
	        this.verdict = source["verdict"];
	        this.rulesetVersion = source["rulesetVersion"];
	        this.judgeModel = source["judgeModel"];
	        this.finishedAt = source["finishedAt"];
	        this.findingCounts = source["findingCounts"];
	        this.findings = this.convertValues(source["findings"], FindingView);
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
	export class ReviewFindingView {
	    scanner: string;
	    severity?: string;
	    confidence?: string;
	    title: string;
	    file?: string;
	    line?: number;
	    excerpt?: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new ReviewFindingView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scanner = source["scanner"];
	        this.severity = source["severity"];
	        this.confidence = source["confidence"];
	        this.title = source["title"];
	        this.file = source["file"];
	        this.line = source["line"];
	        this.excerpt = source["excerpt"];
	        this.detail = source["detail"];
	    }
	}
	export class ReviewAIReviewView {
	    ran: boolean;
	    model?: string;
	    rubricHash?: string;
	    findings: ReviewFindingView[];
	    files: number;
	    notRead: number;
	    failure?: string;
	
	    static createFrom(source: any = {}) {
	        return new ReviewAIReviewView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ran = source["ran"];
	        this.model = source["model"];
	        this.rubricHash = source["rubricHash"];
	        this.findings = this.convertValues(source["findings"], ReviewFindingView);
	        this.files = source["files"];
	        this.notRead = source["notRead"];
	        this.failure = source["failure"];
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
	export class ReviewFileView {
	    file: string;
	    why: string;
	
	    static createFrom(source: any = {}) {
	        return new ReviewFileView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.why = source["why"];
	    }
	}
	
	export class ReviewIssueView {
	    rule: string;
	    error: boolean;
	    path?: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ReviewIssueView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule = source["rule"];
	        this.error = source["error"];
	        this.path = source["path"];
	        this.message = source["message"];
	    }
	}
	export class ReviewReadingView {
	    summary?: string;
	    findings: ReviewFindingView[];
	    notRead: ReviewFileView[];
	    withheld: ReviewFileView[];
	    error?: string;
	    recovered?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ReviewReadingView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = source["summary"];
	        this.findings = this.convertValues(source["findings"], ReviewFindingView);
	        this.notRead = this.convertValues(source["notRead"], ReviewFileView);
	        this.withheld = this.convertValues(source["withheld"], ReviewFileView);
	        this.error = source["error"];
	        this.recovered = source["recovered"];
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
	export class ReviewServerView {
	    ref: string;
	    status: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ReviewServerView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.status = source["status"];
	        this.error = source["error"];
	    }
	}
	export class ReviewRulesView {
	    manifest: ReviewIssueView[];
	    findings: ReviewFindingView[];
	    bundleVersion: string;
	    bundleSource: string;
	    servers: ReviewServerView[];
	
	    static createFrom(source: any = {}) {
	        return new ReviewRulesView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.manifest = this.convertValues(source["manifest"], ReviewIssueView);
	        this.findings = this.convertValues(source["findings"], ReviewFindingView);
	        this.bundleVersion = source["bundleVersion"];
	        this.bundleSource = source["bundleSource"];
	        this.servers = this.convertValues(source["servers"], ReviewServerView);
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
	
	export class ReviewStatusView {
	    key: JevKeyView;
	    modelName: string;
	
	    static createFrom(source: any = {}) {
	        return new ReviewStatusView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = this.convertValues(source["key"], JevKeyView);
	        this.modelName = source["modelName"];
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
	export class ReviewView {
	    runId: string;
	    outDir: string;
	    ref: string;
	    files: number;
	    outcome: string;
	    outcomeWords: string;
	    reason: string;
	    assumes: string;
	    policyFromRegistry: boolean;
	    policyError?: string;
	    publishesUpTo: string;
	    aiCounted: boolean;
	    rules: ReviewRulesView;
	    aiReview: ReviewAIReviewView;
	    reading: ReviewReadingView;
	    notRunHere: string[];
	    notice: string;
	    examinerModel?: string;
	    runError?: string;
	
	    static createFrom(source: any = {}) {
	        return new ReviewView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.outDir = source["outDir"];
	        this.ref = source["ref"];
	        this.files = source["files"];
	        this.outcome = source["outcome"];
	        this.outcomeWords = source["outcomeWords"];
	        this.reason = source["reason"];
	        this.assumes = source["assumes"];
	        this.policyFromRegistry = source["policyFromRegistry"];
	        this.policyError = source["policyError"];
	        this.publishesUpTo = source["publishesUpTo"];
	        this.aiCounted = source["aiCounted"];
	        this.rules = this.convertValues(source["rules"], ReviewRulesView);
	        this.aiReview = this.convertValues(source["aiReview"], ReviewAIReviewView);
	        this.reading = this.convertValues(source["reading"], ReviewReadingView);
	        this.notRunHere = source["notRunHere"];
	        this.notice = source["notice"];
	        this.examinerModel = source["examinerModel"];
	        this.runError = source["runError"];
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
	
	export class RunDefaultsView {
	    timeoutSeconds: number;
	    maxTurns: number;
	    policyPath: string;
	    where: string;
	    outputDir?: string;
	    chatIdleTimeoutSeconds: number;
	    unsafeMode: boolean;
	    unsafeImage?: string;
	    builtinNoLimit: boolean;
	    rememberRuns: boolean;
	    runHistoryCount: number;
	
	    static createFrom(source: any = {}) {
	        return new RunDefaultsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.maxTurns = source["maxTurns"];
	        this.policyPath = source["policyPath"];
	        this.where = source["where"];
	        this.outputDir = source["outputDir"];
	        this.chatIdleTimeoutSeconds = source["chatIdleTimeoutSeconds"];
	        this.unsafeMode = source["unsafeMode"];
	        this.unsafeImage = source["unsafeImage"];
	        this.builtinNoLimit = source["builtinNoLimit"];
	        this.rememberRuns = source["rememberRuns"];
	        this.runHistoryCount = source["runHistoryCount"];
	    }
	}
	export class RunOutputsView {
	    files: OutputFileView[];
	    skipped: number;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RunOutputsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = this.convertValues(source["files"], OutputFileView);
	        this.skipped = source["skipped"];
	        this.truncated = source["truncated"];
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
	export class RunResultView {
	    status: string;
	    exitCode: number;
	    error?: string;
	    runId: string;
	    ref: string;
	    digest: string;
	    resultJson: string;
	    outDir: string;
	    receiptsDir: string;
	    runPlanSha: string;
	    inputTokens: number;
	    outputTokens: number;
	    costUsd: number;
	    wallMs: number;
	    turns: number;
	    attestationState: string;
	    policy?: receipt.Policy;
	    knowledge?: string;
	    unsafeExec?: string;
	    decode?: string;
	    exitClass?: string;
	    stage?: string;
	    reason?: receipt.Reason;
	
	    static createFrom(source: any = {}) {
	        return new RunResultView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.exitCode = source["exitCode"];
	        this.error = source["error"];
	        this.runId = source["runId"];
	        this.ref = source["ref"];
	        this.digest = source["digest"];
	        this.resultJson = source["resultJson"];
	        this.outDir = source["outDir"];
	        this.receiptsDir = source["receiptsDir"];
	        this.runPlanSha = source["runPlanSha"];
	        this.inputTokens = source["inputTokens"];
	        this.outputTokens = source["outputTokens"];
	        this.costUsd = source["costUsd"];
	        this.wallMs = source["wallMs"];
	        this.turns = source["turns"];
	        this.attestationState = source["attestationState"];
	        this.policy = this.convertValues(source["policy"], receipt.Policy);
	        this.knowledge = source["knowledge"];
	        this.unsafeExec = source["unsafeExec"];
	        this.decode = source["decode"];
	        this.exitClass = source["exitClass"];
	        this.stage = source["stage"];
	        this.reason = this.convertValues(source["reason"], receipt.Reason);
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
	export class SignedOutView {
	    available: boolean;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new SignedOutView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.reason = source["reason"];
	    }
	}
	export class ScheduleAvailabilityView {
	    available: boolean;
	    reason?: string;
	    zone: string;
	    signedOut: SignedOutView;
	    lingerOn: boolean;
	    installCLI?: string;
	
	    static createFrom(source: any = {}) {
	        return new ScheduleAvailabilityView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.reason = source["reason"];
	        this.zone = source["zone"];
	        this.signedOut = this.convertValues(source["signedOut"], SignedOutView);
	        this.lingerOn = source["lingerOn"];
	        this.installCLI = source["installCLI"];
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
	export class ScheduleMadeView {
	    id: string;
	
	    static createFrom(source: any = {}) {
	        return new ScheduleMadeView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	    }
	}
	export class ScheduledCardView {
	    reason?: string;
	    schedules: number;
	    month: schedules.Fire[];
	    hostedMonth: CalendarFire[];
	    hostedSchedules: number;
	    hostedReason?: string;
	    next: CalendarFire[];
	
	    static createFrom(source: any = {}) {
	        return new ScheduledCardView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reason = source["reason"];
	        this.schedules = source["schedules"];
	        this.month = this.convertValues(source["month"], schedules.Fire);
	        this.hostedMonth = this.convertValues(source["hostedMonth"], CalendarFire);
	        this.hostedSchedules = source["hostedSchedules"];
	        this.hostedReason = source["hostedReason"];
	        this.next = this.convertValues(source["next"], CalendarFire);
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
	export class SchedulesView {
	    schedules: schedules.Item[];
	    running: string[];
	    resealProblem?: string;
	
	    static createFrom(source: any = {}) {
	        return new SchedulesView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schedules = this.convertValues(source["schedules"], schedules.Item);
	        this.running = source["running"];
	        this.resealProblem = source["resealProblem"];
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
	export class SearchHitView {
	    namespace: string;
	    name: string;
	    modes: string[];
	    summary: string;
	    installs30d: number;
	    quarantine?: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchHitView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.modes = source["modes"];
	        this.summary = source["summary"];
	        this.installs30d = source["installs30d"];
	        this.quarantine = source["quarantine"];
	    }
	}
	export class SearchKeyTestView {
	    ok: boolean;
	    message: string;
	    from?: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchKeyTestView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	        this.from = source["from"];
	    }
	}
	export class SearchProviderView {
	    name: string;
	    label: string;
	    envName: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchProviderView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	        this.envName = source["envName"];
	    }
	}
	export class SearchKeyView {
	    provider?: string;
	    providerLabel?: string;
	    stored: boolean;
	    source?: string;
	    envName?: string;
	    envSet: boolean;
	    ready: boolean;
	    keyStoreErr?: string;
	    providers: SearchProviderView[];
	
	    static createFrom(source: any = {}) {
	        return new SearchKeyView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.providerLabel = source["providerLabel"];
	        this.stored = source["stored"];
	        this.source = source["source"];
	        this.envName = source["envName"];
	        this.envSet = source["envSet"];
	        this.ready = source["ready"];
	        this.keyStoreErr = source["keyStoreErr"];
	        this.providers = this.convertValues(source["providers"], SearchProviderView);
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
	
	export class SearchQuery {
	    q: string;
	    mode?: string;
	    capability?: string;
	    license?: string;
	    sort?: string;
	    limit?: number;
	    offset?: number;
	
	    static createFrom(source: any = {}) {
	        return new SearchQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.q = source["q"];
	        this.mode = source["mode"];
	        this.capability = source["capability"];
	        this.license = source["license"];
	        this.sort = source["sort"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	    }
	}
	export class SearchView {
	    total?: number;
	    results: SearchHitView[];
	    facets?: {[key: string]: any};
	
	    static createFrom(source: any = {}) {
	        return new SearchView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.results = this.convertValues(source["results"], SearchHitView);
	        this.facets = source["facets"];
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
	export class SelfUpdateView {
	    channel: string;
	    current: string;
	    enabled: boolean;
	    checked?: string;
	    available?: selfupdate.Release;
	    err?: string;
	    downloadUrl?: string;
	    nothing?: boolean;
	    canApply: boolean;
	    cannotApplyWhy?: string;
	    onQuit: boolean;
	    applying?: boolean;
	    dismissed?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SelfUpdateView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.channel = source["channel"];
	        this.current = source["current"];
	        this.enabled = source["enabled"];
	        this.checked = source["checked"];
	        this.available = this.convertValues(source["available"], selfupdate.Release);
	        this.err = source["err"];
	        this.downloadUrl = source["downloadUrl"];
	        this.nothing = source["nothing"];
	        this.canApply = source["canApply"];
	        this.cannotApplyWhy = source["cannotApplyWhy"];
	        this.onQuit = source["onQuit"];
	        this.applying = source["applying"];
	        this.dismissed = source["dismissed"];
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
	export class SessionView {
	    ref: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	    }
	}
	export class SettingsView {
	    defaultRegistry: string;
	    registries: RegistryView[];
	    configPath: string;
	    onboarded: boolean;
	    unsafeMode: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SettingsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultRegistry = source["defaultRegistry"];
	        this.registries = this.convertValues(source["registries"], RegistryView);
	        this.configPath = source["configPath"];
	        this.onboarded = source["onboarded"];
	        this.unsafeMode = source["unsafeMode"];
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
	export class SetupStatusView {
	    dockerOk: boolean;
	    dockerErr?: string;
	    policyPath: string;
	    policyExists: boolean;
	    providerSetUp: boolean;
	    provider: string;
	    model: string;
	    apiKeyEnv: string;
	    apiKeySource?: string;
	    keysStored: string[];
	    keyStoreErr?: string;
	    apiKeyKind?: string;
	    keyNotice?: string;
	    baseUrl?: string;
	    imagesReady: boolean;
	    timeoutSeconds: number;
	    maxTurns: number;
	    keepProfilesByDefault: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SetupStatusView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dockerOk = source["dockerOk"];
	        this.dockerErr = source["dockerErr"];
	        this.policyPath = source["policyPath"];
	        this.policyExists = source["policyExists"];
	        this.providerSetUp = source["providerSetUp"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.apiKeyEnv = source["apiKeyEnv"];
	        this.apiKeySource = source["apiKeySource"];
	        this.keysStored = source["keysStored"];
	        this.keyStoreErr = source["keyStoreErr"];
	        this.apiKeyKind = source["apiKeyKind"];
	        this.keyNotice = source["keyNotice"];
	        this.baseUrl = source["baseUrl"];
	        this.imagesReady = source["imagesReady"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.maxTurns = source["maxTurns"];
	        this.keepProfilesByDefault = source["keepProfilesByDefault"];
	    }
	}
	export class SignInResult {
	    user: UserView;
	    tokenStore: string;
	    pinNote: string;
	    knowledgeRewrapped?: string[];
	    knowledgeNote?: string;
	
	    static createFrom(source: any = {}) {
	        return new SignInResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user = this.convertValues(source["user"], UserView);
	        this.tokenStore = source["tokenStore"];
	        this.pinNote = source["pinNote"];
	        this.knowledgeRewrapped = source["knowledgeRewrapped"];
	        this.knowledgeNote = source["knowledgeNote"];
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
	export class SignInView {
	    signedIn?: run.SignedIn;
	    cancelled?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SignInView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.signedIn = this.convertValues(source["signedIn"], run.SignedIn);
	        this.cancelled = source["cancelled"];
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
	
	export class SourceKeyView {
	    problem?: string;
	    host?: string;
	    shown?: string;
	    saved: boolean;
	    keychainOff: boolean;
	    keyStoreErr?: string;
	
	    static createFrom(source: any = {}) {
	        return new SourceKeyView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.problem = source["problem"];
	        this.host = source["host"];
	        this.shown = source["shown"];
	        this.saved = source["saved"];
	        this.keychainOff = source["keychainOff"];
	        this.keyStoreErr = source["keyStoreErr"];
	    }
	}
	export class StarAskView {
	    ask: boolean;
	    name?: string;
	
	    static createFrom(source: any = {}) {
	        return new StarAskView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ask = source["ask"];
	        this.name = source["name"];
	    }
	}
	export class UnsafeNoticeView {
	    on: boolean;
	    line?: string;
	    unplaced: string[];
	
	    static createFrom(source: any = {}) {
	        return new UnsafeNoticeView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.on = source["on"];
	        this.line = source["line"];
	        this.unplaced = source["unplaced"];
	    }
	}
	
	
	export class VoiceHostedAvailabilityView {
	    offered: boolean;
	    available: boolean;
	    reason?: string;
	    remedy?: string;
	    upgradeUrl?: string;
	    credits: number;
	    holdCredits: number;
	    limited: boolean;
	    max: number;
	    left: number;
	    aiReviewCredits: number;
	    balance: number;
	    balanceKnown: boolean;
	    pool?: string;
	    maxInputFiles: number;
	    maxInputBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new VoiceHostedAvailabilityView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offered = source["offered"];
	        this.available = source["available"];
	        this.reason = source["reason"];
	        this.remedy = source["remedy"];
	        this.upgradeUrl = source["upgradeUrl"];
	        this.credits = source["credits"];
	        this.holdCredits = source["holdCredits"];
	        this.limited = source["limited"];
	        this.max = source["max"];
	        this.left = source["left"];
	        this.aiReviewCredits = source["aiReviewCredits"];
	        this.balance = source["balance"];
	        this.balanceKnown = source["balanceKnown"];
	        this.pool = source["pool"];
	        this.maxInputFiles = source["maxInputFiles"];
	        this.maxInputBytes = source["maxInputBytes"];
	    }
	}
	export class VoiceHostedView {
	    id: string;
	    op: string;
	    status: string;
	    state: string;
	    statusLine: string;
	    cancelRequested: boolean;
	    outcomeLine?: string;
	    outcomeTone?: string;
	    error?: string;
	    holdCredits: number;
	    settledCredits: number;
	    settled: boolean;
	    applied: boolean;
	    bytes?: number;
	    note?: string;
	    final: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VoiceHostedView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.op = source["op"];
	        this.status = source["status"];
	        this.state = source["state"];
	        this.statusLine = source["statusLine"];
	        this.cancelRequested = source["cancelRequested"];
	        this.outcomeLine = source["outcomeLine"];
	        this.outcomeTone = source["outcomeTone"];
	        this.error = source["error"];
	        this.holdCredits = source["holdCredits"];
	        this.settledCredits = source["settledCredits"];
	        this.settled = source["settled"];
	        this.applied = source["applied"];
	        this.bytes = source["bytes"];
	        this.note = source["note"];
	        this.final = source["final"];
	    }
	}
	export class VoiceHostedStartView {
	    run?: VoiceHostedView;
	    refusal?: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new VoiceHostedStartView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run = this.convertValues(source["run"], VoiceHostedView);
	        this.refusal = source["refusal"];
	        this.detail = source["detail"];
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
	
	export class VoiceRestoreView {
	    account?: voice.Account;
	    inReview: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VoiceRestoreView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.account = this.convertValues(source["account"], voice.Account);
	        this.inReview = source["inReview"];
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
	export class VoiceSaveView {
	    voice?: voice.Local;
	    created: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VoiceSaveView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.voice = this.convertValues(source["voice"], voice.Local);
	        this.created = source["created"];
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
	export class VoiceView {
	    registry: string;
	    local?: voice.Local;
	    signedIn: boolean;
	    accountOffered: boolean;
	    account?: voice.Account;
	    accountError?: string;
	    model: string;
	    reviewCredits: number;
	    attachEnabled: boolean;
	    sync: voice.Sync;
	
	    static createFrom(source: any = {}) {
	        return new VoiceView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registry = source["registry"];
	        this.local = this.convertValues(source["local"], voice.Local);
	        this.signedIn = source["signedIn"];
	        this.accountOffered = source["accountOffered"];
	        this.account = this.convertValues(source["account"], voice.Account);
	        this.accountError = source["accountError"];
	        this.model = source["model"];
	        this.reviewCredits = source["reviewCredits"];
	        this.attachEnabled = source["attachEnabled"];
	        this.sync = this.convertValues(source["sync"], voice.Sync);
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

}

export namespace manifest {
	
	export class CapabilityDiff {
	    widened?: string[];
	    narrowed?: string[];
	
	    static createFrom(source: any = {}) {
	        return new CapabilityDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.widened = source["widened"];
	        this.narrowed = source["narrowed"];
	    }
	}
	export class MCPOAuthFile {
	    path: string;
	    template: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPOAuthFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.template = source["template"];
	    }
	}
	export class MCPOAuth {
	    issuer: string;
	    scopes?: string[];
	    files?: MCPOAuthFile[];
	    env?: {[key: string]: string};
	
	    static createFrom(source: any = {}) {
	        return new MCPOAuth(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.issuer = source["issuer"];
	        this.scopes = source["scopes"];
	        this.files = this.convertValues(source["files"], MCPOAuthFile);
	        this.env = source["env"];
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

}

export namespace mcpview {
	
	export class EnvVar {
	    name: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new EnvVar(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class Server {
	    id: string;
	    tools: string[];
	    env: EnvVar[];
	    remote?: boolean;
	    signsIn?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.tools = source["tools"];
	        this.env = this.convertValues(source["env"], EnvVar);
	        this.remote = source["remote"];
	        this.signsIn = source["signsIn"];
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

}

export namespace mcpvuln {
	
	export class Advisory {
	    id: string;
	    severity: string;
	    rating: string;
	    cvssScore?: number;
	    refuses: boolean;
	    summary: string;
	    fixedIn: string[];
	
	    static createFrom(source: any = {}) {
	        return new Advisory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.severity = source["severity"];
	        this.rating = source["rating"];
	        this.cvssScore = source["cvssScore"];
	        this.refuses = source["refuses"];
	        this.summary = source["summary"];
	        this.fixedIn = source["fixedIn"];
	    }
	}
	export class Status {
	    ref: string;
	    status: string;
	    maxSeverity?: string;
	    rating?: string;
	    advisories: Advisory[];
	    checkedAt?: string;
	    error?: string;
	    refuses: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.status = source["status"];
	        this.maxSeverity = source["maxSeverity"];
	        this.rating = source["rating"];
	        this.advisories = this.convertValues(source["advisories"], Advisory);
	        this.checkedAt = source["checkedAt"];
	        this.error = source["error"];
	        this.refuses = source["refuses"];
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

}

export namespace receipt {
	
	export class Policy {
	    path: string;
	    source: string;
	    sha256: string;
	
	    static createFrom(source: any = {}) {
	        return new Policy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.source = source["source"];
	        this.sha256 = source["sha256"];
	    }
	}
	export class Reason {
	    code: string;
	    names?: string[];
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new Reason(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.names = source["names"];
	        this.detail = source["detail"];
	    }
	}

}

export namespace remote {
	
	export class Finding {
	    scanner: string;
	    severity: string;
	    title: string;
	    line?: number;
	    question?: string;
	    score?: number;
	    flags: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Finding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scanner = source["scanner"];
	        this.severity = source["severity"];
	        this.title = source["title"];
	        this.line = source["line"];
	        this.question = source["question"];
	        this.score = source["score"];
	        this.flags = source["flags"];
	    }
	}

}

export namespace reportdesign {
	
	export class Logo {
	    png: string;
	
	    static createFrom(source: any = {}) {
	        return new Logo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.png = source["png"];
	    }
	}
	export class Footer {
	    pageNumbers: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new Footer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pageNumbers = source["pageNumbers"];
	        this.text = source["text"];
	    }
	}
	export class Tables {
	    tinted: boolean;
	    banded: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Tables(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tinted = source["tinted"];
	        this.banded = source["banded"];
	    }
	}
	export class Fonts {
	    headings: string;
	    body: string;
	    code: string;
	
	    static createFrom(source: any = {}) {
	        return new Fonts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.headings = source["headings"];
	        this.body = source["body"];
	        this.code = source["code"];
	    }
	}
	export class Design {
	    agentryTheme: number;
	    name: string;
	    basedOn: string;
	    version: number;
	    paper: string;
	    fonts: Fonts;
	    accent: string;
	    titleBlock: string;
	    headings: string;
	    tables: Tables;
	    quotes: string;
	    footer: Footer;
	    logo: Logo;
	
	    static createFrom(source: any = {}) {
	        return new Design(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agentryTheme = source["agentryTheme"];
	        this.name = source["name"];
	        this.basedOn = source["basedOn"];
	        this.version = source["version"];
	        this.paper = source["paper"];
	        this.fonts = this.convertValues(source["fonts"], Fonts);
	        this.accent = source["accent"];
	        this.titleBlock = source["titleBlock"];
	        this.headings = source["headings"];
	        this.tables = this.convertValues(source["tables"], Tables);
	        this.quotes = source["quotes"];
	        this.footer = this.convertValues(source["footer"], Footer);
	        this.logo = this.convertValues(source["logo"], Logo);
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
	
	
	

}

export namespace run {
	
	export class KeptClient {
	    issuer: string;
	    clientId: string;
	    secretIn?: string;
	
	    static createFrom(source: any = {}) {
	        return new KeptClient(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.issuer = source["issuer"];
	        this.clientId = source["clientId"];
	        this.secretIn = source["secretIn"];
	    }
	}
	export class KeptConnection {
	    ref: string;
	    digest: string;
	    server: string;
	    issuer: string;
	    client: string;
	    scopes: string[];
	    state: string;
	    broken?: string;
	    label?: string;
	    createdAt: string;
	    lastUsedAt?: string;
	    keptIn: string;
	    sharesGrantWith: string[];
	
	    static createFrom(source: any = {}) {
	        return new KeptConnection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.digest = source["digest"];
	        this.server = source["server"];
	        this.issuer = source["issuer"];
	        this.client = source["client"];
	        this.scopes = source["scopes"];
	        this.state = source["state"];
	        this.broken = source["broken"];
	        this.label = source["label"];
	        this.createdAt = source["createdAt"];
	        this.lastUsedAt = source["lastUsedAt"];
	        this.keptIn = source["keptIn"];
	        this.sharesGrantWith = source["sharesGrantWith"];
	    }
	}
	export class ModelSignIn {
	    vendor: string;
	    provider: string;
	    kind: string;
	    connected: boolean;
	    account?: string;
	    broken?: string;
	    keyExpiresAt?: string;
	    createdAt: string;
	    image: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelSignIn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vendor = source["vendor"];
	        this.provider = source["provider"];
	        this.kind = source["kind"];
	        this.connected = source["connected"];
	        this.account = source["account"];
	        this.broken = source["broken"];
	        this.keyExpiresAt = source["keyExpiresAt"];
	        this.createdAt = source["createdAt"];
	        this.image = source["image"];
	    }
	}
	export class OwnClientGuide {
	    issuer: string;
	    vendor?: string;
	    known: boolean;
	    steps: string[];
	    note?: string;
	    docs?: string;
	
	    static createFrom(source: any = {}) {
	        return new OwnClientGuide(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.issuer = source["issuer"];
	        this.vendor = source["vendor"];
	        this.known = source["known"];
	        this.steps = source["steps"];
	        this.note = source["note"];
	        this.docs = source["docs"];
	    }
	}
	export class ServerConnection {
	    server: string;
	    what: string;
	    issuer?: string;
	    scopes: string[];
	    state: string;
	    broken?: string;
	    label?: string;
	    client?: string;
	    granted: string[];
	    createdAt?: string;
	    lastUsedAt?: string;
	    unsupported?: string;
	    sharesGrantWith: string[];
	
	    static createFrom(source: any = {}) {
	        return new ServerConnection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server = source["server"];
	        this.what = source["what"];
	        this.issuer = source["issuer"];
	        this.scopes = source["scopes"];
	        this.state = source["state"];
	        this.broken = source["broken"];
	        this.label = source["label"];
	        this.client = source["client"];
	        this.granted = source["granted"];
	        this.createdAt = source["createdAt"];
	        this.lastUsedAt = source["lastUsedAt"];
	        this.unsupported = source["unsupported"];
	        this.sharesGrantWith = source["sharesGrantWith"];
	    }
	}
	export class ReleaseConnections {
	    ref: string;
	    digest: string;
	    servers: ServerConnection[];
	
	    static createFrom(source: any = {}) {
	        return new ReleaseConnections(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.digest = source["digest"];
	        this.servers = this.convertValues(source["servers"], ServerConnection);
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
	
	export class SignedIn {
	    vendor: string;
	    account?: string;
	    models: string[];
	
	    static createFrom(source: any = {}) {
	        return new SignedIn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vendor = source["vendor"];
	        this.account = source["account"];
	        this.models = source["models"];
	    }
	}

}

export namespace schedules {
	
	export class SignedOut {
	    available: boolean;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new SignedOut(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.reason = source["reason"];
	    }
	}
	export class Refusal {
	    kind: string;
	    message: string;
	    lift?: string;
	
	    static createFrom(source: any = {}) {
	        return new Refusal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.message = source["message"];
	        this.lift = source["lift"];
	    }
	}
	export class Check {
	    ref: string;
	    version: string;
	    digest: string;
	    modes: string[];
	    ready: boolean;
	    gaps: string[];
	    needsConsent: boolean;
	    consentReason?: string;
	    refusals: Refusal[];
	    installed: boolean;
	    signedOut: SignedOut;
	    describe?: string;
	    next?: string;
	
	    static createFrom(source: any = {}) {
	        return new Check(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.version = source["version"];
	        this.digest = source["digest"];
	        this.modes = source["modes"];
	        this.ready = source["ready"];
	        this.gaps = source["gaps"];
	        this.needsConsent = source["needsConsent"];
	        this.consentReason = source["consentReason"];
	        this.refusals = this.convertValues(source["refusals"], Refusal);
	        this.installed = source["installed"];
	        this.signedOut = this.convertValues(source["signedOut"], SignedOut);
	        this.describe = source["describe"];
	        this.next = source["next"];
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
	export class When {
	    kind: string;
	    date?: string;
	    time: string;
	    days?: string[];
	    monthDay?: number;
	    end?: string;
	    until?: string;
	    times?: number;
	    zone?: string;
	
	    static createFrom(source: any = {}) {
	        return new When(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.date = source["date"];
	        this.time = source["time"];
	        this.days = source["days"];
	        this.monthDay = source["monthDay"];
	        this.end = source["end"];
	        this.until = source["until"];
	        this.times = source["times"];
	        this.zone = source["zone"];
	    }
	}
	export class Edit {
	    name?: string;
	    when?: When;
	    signedOut?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Edit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.when = this.convertValues(source["when"], When);
	        this.signedOut = source["signedOut"];
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
	export class Fire {
	    scheduleId: string;
	    name: string;
	    ref: string;
	    at: string;
	    outcome: string;
	    manual?: boolean;
	    started?: string;
	    ended?: string;
	    runId?: string;
	    exitCode?: number;
	    exitClass?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new Fire(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scheduleId = source["scheduleId"];
	        this.name = source["name"];
	        this.ref = source["ref"];
	        this.at = source["at"];
	        this.outcome = source["outcome"];
	        this.manual = source["manual"];
	        this.started = source["started"];
	        this.ended = source["ended"];
	        this.runId = source["runId"];
	        this.exitCode = source["exitCode"];
	        this.exitClass = source["exitClass"];
	        this.message = source["message"];
	    }
	}
	export class Item {
	    id: string;
	    name: string;
	    ref: string;
	    version: string;
	    registry?: string;
	    local?: boolean;
	    describe: string;
	    when: When;
	    outDir: string;
	    paused?: boolean;
	    ended?: boolean;
	    signedOut?: boolean;
	    keysSealed?: boolean;
	    runs?: number;
	    next?: string;
	    last?: Fire;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.ref = source["ref"];
	        this.version = source["version"];
	        this.registry = source["registry"];
	        this.local = source["local"];
	        this.describe = source["describe"];
	        this.when = this.convertValues(source["when"], When);
	        this.outDir = source["outDir"];
	        this.paused = source["paused"];
	        this.ended = source["ended"];
	        this.signedOut = source["signedOut"];
	        this.keysSealed = source["keysSealed"];
	        this.runs = source["runs"];
	        this.next = source["next"];
	        this.last = this.convertValues(source["last"], Fire);
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
	
	export class Request {
	    registry: string;
	    ref: string;
	    name?: string;
	    outDir: string;
	    task?: string;
	    files?: string[];
	    knowledge?: string[];
	    intake?: {[key: string]: string};
	    voice: boolean;
	    timeoutSeconds?: number;
	    maxTurns: number;
	    allowUnsigned?: boolean;
	    allowUnscanned?: boolean;
	    allowVulnerableMcp?: boolean;
	    signedOut?: boolean;
	    when: When;
	
	    static createFrom(source: any = {}) {
	        return new Request(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registry = source["registry"];
	        this.ref = source["ref"];
	        this.name = source["name"];
	        this.outDir = source["outDir"];
	        this.task = source["task"];
	        this.files = source["files"];
	        this.knowledge = source["knowledge"];
	        this.intake = source["intake"];
	        this.voice = source["voice"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.maxTurns = source["maxTurns"];
	        this.allowUnsigned = source["allowUnsigned"];
	        this.allowUnscanned = source["allowUnscanned"];
	        this.allowVulnerableMcp = source["allowVulnerableMcp"];
	        this.signedOut = source["signedOut"];
	        this.when = this.convertValues(source["when"], When);
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
	

}

export namespace selfupdate {
	
	export class Release {
	    version: string;
	    commit: string;
	    signedBy: string;
	    retired: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Release(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.commit = source["commit"];
	        this.signedBy = source["signedBy"];
	        this.retired = source["retired"];
	    }
	}

}

export namespace settings {
	
	export class HomePin {
	    ref: string;
	    registry?: string;
	
	    static createFrom(source: any = {}) {
	        return new HomePin(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.registry = source["registry"];
	    }
	}
	export class HomeCard {
	    kind: string;
	    hidden?: boolean;
	    size?: string;
	    pins?: HomePin[];
	
	    static createFrom(source: any = {}) {
	        return new HomeCard(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.hidden = source["hidden"];
	        this.size = source["size"];
	        this.pins = this.convertValues(source["pins"], HomePin);
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

}

export namespace trust {
	
	export class KeyChange {
	    added?: string[];
	    removed?: string[];
	    replaced?: string[];
	
	    static createFrom(source: any = {}) {
	        return new KeyChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added = source["added"];
	        this.removed = source["removed"];
	        this.replaced = source["replaced"];
	    }
	}
	export class AttestationResult {
	    state: string;
	    detail: string;
	    keyId?: string;
	    agent?: string;
	    version?: string;
	    rulesetVersion?: string;
	    reportDigest?: string;
	    judgeModel?: string;
	    source?: string;
	    firstUse?: boolean;
	    keyRotated?: boolean;
	    keyChange?: KeyChange;
	    keyRetired?: boolean;
	    keyRetiredAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new AttestationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.detail = source["detail"];
	        this.keyId = source["keyId"];
	        this.agent = source["agent"];
	        this.version = source["version"];
	        this.rulesetVersion = source["rulesetVersion"];
	        this.reportDigest = source["reportDigest"];
	        this.judgeModel = source["judgeModel"];
	        this.source = source["source"];
	        this.firstUse = source["firstUse"];
	        this.keyRotated = source["keyRotated"];
	        this.keyChange = this.convertValues(source["keyChange"], KeyChange);
	        this.keyRetired = source["keyRetired"];
	        this.keyRetiredAt = source["keyRetiredAt"];
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

}

export namespace validate {
	
	export class InventoryFinding {
	    scanner: string;
	    severity: string;
	    title: string;
	    filePath: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new InventoryFinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scanner = source["scanner"];
	        this.severity = source["severity"];
	        this.title = source["title"];
	        this.filePath = source["filePath"];
	        this.detail = source["detail"];
	    }
	}
	export class Issue {
	    rule: string;
	    path?: string;
	    message: string;
	    warning: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Issue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule = source["rule"];
	        this.path = source["path"];
	        this.message = source["message"];
	        this.warning = source["warning"];
	    }
	}
	export class ScanFinding {
	    ruleId: string;
	    scanner: string;
	    severity: string;
	    confidence: string;
	    title: string;
	    filePath: string;
	    lineStart: number;
	    lineEnd: number;
	    excerpt: string;
	    taxonomy?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ScanFinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ruleId = source["ruleId"];
	        this.scanner = source["scanner"];
	        this.severity = source["severity"];
	        this.confidence = source["confidence"];
	        this.title = source["title"];
	        this.filePath = source["filePath"];
	        this.lineStart = source["lineStart"];
	        this.lineEnd = source["lineEnd"];
	        this.excerpt = source["excerpt"];
	        this.taxonomy = source["taxonomy"];
	    }
	}
	export class Result {
	    dir: string;
	    manifestName?: string;
	    manifestModes?: string[];
	    manifestVersion?: string;
	    manifestError?: string;
	    issues: Issue[];
	    inventory: InventoryFinding[];
	    findings: ScanFinding[];
	    bundleVersion: string;
	    bundleSource: string;
	    fileCount: number;
	    readErrors?: string[];
	    errorCount: number;
	    warnCount: number;
	    inventoryFailing: number;
	    passed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.manifestName = source["manifestName"];
	        this.manifestModes = source["manifestModes"];
	        this.manifestVersion = source["manifestVersion"];
	        this.manifestError = source["manifestError"];
	        this.issues = this.convertValues(source["issues"], Issue);
	        this.inventory = this.convertValues(source["inventory"], InventoryFinding);
	        this.findings = this.convertValues(source["findings"], ScanFinding);
	        this.bundleVersion = source["bundleVersion"];
	        this.bundleSource = source["bundleSource"];
	        this.fileCount = source["fileCount"];
	        this.readErrors = source["readErrors"];
	        this.errorCount = source["errorCount"];
	        this.warnCount = source["warnCount"];
	        this.inventoryFailing = source["inventoryFailing"];
	        this.passed = source["passed"];
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

}

export namespace voice {
	
	export class Version {
	    review: string;
	    source: string;
	    digest: string;
	    bytes: number;
	    text: string;
	    verdict?: string;
	    findings?: remote.Finding[];
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    reviewedAt?: any;
	    questionSet?: string;
	
	    static createFrom(source: any = {}) {
	        return new Version(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.review = source["review"];
	        this.source = source["source"];
	        this.digest = source["digest"];
	        this.bytes = source["bytes"];
	        this.text = source["text"];
	        this.verdict = source["verdict"];
	        this.findings = this.convertValues(source["findings"], remote.Finding);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.reviewedAt = this.convertValues(source["reviewedAt"], null);
	        this.questionSet = source["questionSet"];
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
	export class Account {
	    exists: boolean;
	    alwaysAttach: boolean;
	    live?: Version;
	    pending?: Version;
	    previous?: Version;
	    questionSet?: string;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exists = source["exists"];
	        this.alwaysAttach = source["alwaysAttach"];
	        this.live = this.convertValues(source["live"], Version);
	        this.pending = this.convertValues(source["pending"], Version);
	        this.previous = this.convertValues(source["previous"], Version);
	        this.questionSet = source["questionSet"];
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
	export class Local {
	    exists: boolean;
	    text: string;
	    digest: string;
	    bytes: number;
	    source: string;
	    phrase: string;
	    alwaysAttach: boolean;
	    created: string;
	    updated: string;
	    problem?: string;
	
	    static createFrom(source: any = {}) {
	        return new Local(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exists = source["exists"];
	        this.text = source["text"];
	        this.digest = source["digest"];
	        this.bytes = source["bytes"];
	        this.source = source["source"];
	        this.phrase = source["phrase"];
	        this.alwaysAttach = source["alwaysAttach"];
	        this.created = source["created"];
	        this.updated = source["updated"];
	        this.problem = source["problem"];
	    }
	}
	export class BuildResult {
	    voice?: Local;
	    created: boolean;
	    samples?: string[];
	    skipped?: string[];
	
	    static createFrom(source: any = {}) {
	        return new BuildResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.voice = this.convertValues(source["voice"], Local);
	        this.created = source["created"];
	        this.samples = source["samples"];
	        this.skipped = source["skipped"];
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
	
	export class Sync {
	    downloadSame: boolean;
	    uploadSame: boolean;
	    localAfterLive: boolean;
	    accountAfterLocal: boolean;
	    inReview: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Sync(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.downloadSame = source["downloadSame"];
	        this.uploadSame = source["uploadSame"];
	        this.localAfterLive = source["localAfterLive"];
	        this.accountAfterLocal = source["accountAfterLocal"];
	        this.inReview = source["inReview"];
	    }
	}

}

