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
	export class MCPServerRow {
	    source: string;
	    package: string;
	    version: string;
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
	    dataAccess: string[];
	    mcpServers: MCPServerRow[];
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
	        this.dataAccess = source["dataAccess"];
	        this.mcpServers = this.convertValues(source["mcpServers"], MCPServerRow);
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
	    }
	}
	
	export class SummaryServer {
	    ref: string;
	    tools: string[];
	    hosts: SummaryHost[];
	    keys: string[];
	    config: string[];
	
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
	
	    static createFrom(source: any = {}) {
	        return new SummaryKey(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.help = source["help"];
	        this.usedBy = source["usedBy"];
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
	    origin: string;
	    present: boolean;
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
	        this.origin = source["origin"];
	        this.present = source["present"];
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
	        this.knowledge = source["knowledge"];
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
	    skipped?: string[];
	    dropped?: Dropped[];
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
	        this.skipped = source["skipped"];
	        this.dropped = this.convertValues(source["dropped"], Dropped);
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
	    seats?: Seats;
	    memberOf?: string;
	    balance: number;
	    allowance: number;
	    submissionCost: number;
	    periodEnd: string;
	    pendingTier?: string;
	    // Go type: struct { Max int64 "json:\"max\""; Used int64 "json:\"used\"" }
	    privateAgents: any;
	    upgrade: UpgradeRef;
	
	    static createFrom(source: any = {}) {
	        return new SubscriptionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tier = source["tier"];
	        this.standing = source["standing"];
	        this.unmetered = source["unmetered"];
	        this.seats = this.convertValues(source["seats"], Seats);
	        this.memberOf = source["memberOf"];
	        this.balance = source["balance"];
	        this.allowance = source["allowance"];
	        this.submissionCost = source["submissionCost"];
	        this.periodEnd = source["periodEnd"];
	        this.pendingTier = source["pendingTier"];
	        this.privateAgents = this.convertValues(source["privateAgents"], Object);
	        this.upgrade = this.convertValues(source["upgrade"], UpgradeRef);
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
	export class UserView {
	    handle: string;
	    displayName: string;
	    email: string;
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
	export class AccountView {
	    user: UserView;
	    subscription?: SubscriptionView;
	    subscriptionError?: string;
	
	    static createFrom(source: any = {}) {
	        return new AccountView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user = this.convertValues(source["user"], UserView);
	        this.subscription = this.convertValues(source["subscription"], SubscriptionView);
	        this.subscriptionError = source["subscriptionError"];
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
	export class AgentView {
	    namespace: string;
	    name: string;
	    modes: string[];
	    summary: string;
	    visibility: string;
	    quarantine?: string;
	    createdAt: string;
	    gitHttpUrl?: string;
	
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
	    balance: number;
	    balanceKnown: boolean;
	    pool?: string;
	    unmetered: boolean;
	    short: boolean;
	
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
	        this.balance = source["balance"];
	        this.balanceKnown = source["balanceKnown"];
	        this.pool = source["pool"];
	        this.unmetered = source["unmetered"];
	        this.short = source["short"];
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
	
	    static createFrom(source: any = {}) {
	        return new DescribeStatusView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = this.convertValues(source["key"], JevKeyView);
	        this.search = source["search"];
	        this.searchSentence = source["searchSentence"];
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
	export class HostedCapsView {
	    timeoutSeconds: number;
	    maxTurns: number;
	    maxTokens: number;
	
	    static createFrom(source: any = {}) {
	        return new HostedCapsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.maxTurns = source["maxTurns"];
	        this.maxTokens = source["maxTokens"];
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
	    inputTokensPerCredit: number;
	    outputTokensPerCredit: number;
	    caps?: HostedCapsView;
	    balance: number;
	    balanceKnown: boolean;
	    knowledgeAttach: boolean;
	    maxPacks?: number;
	    knowledgeNote?: string;
	    maxInputFiles: number;
	    maxInputBytes: number;
	    declaredSecrets?: string[];
	
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
	        this.inputTokensPerCredit = source["inputTokensPerCredit"];
	        this.outputTokensPerCredit = source["outputTokensPerCredit"];
	        this.caps = this.convertValues(source["caps"], HostedCapsView);
	        this.balance = source["balance"];
	        this.balanceKnown = source["balanceKnown"];
	        this.knowledgeAttach = source["knowledgeAttach"];
	        this.maxPacks = source["maxPacks"];
	        this.knowledgeNote = source["knowledgeNote"];
	        this.maxInputFiles = source["maxInputFiles"];
	        this.maxInputBytes = source["maxInputBytes"];
	        this.declaredSecrets = source["declaredSecrets"];
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
	    inputTokens: number;
	    outputTokens: number;
	    totalTokens: number;
	    costUsd: number;
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
	        this.inputTokens = source["inputTokens"];
	        this.outputTokens = source["outputTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.costUsd = source["costUsd"];
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
	export class HostedStartView {
	    run?: HostedRunView;
	    refusal?: string;
	    detail?: string;
	    balance?: number;
	    hold?: number;
	    resetsAt?: string;
	    missing?: string[];
	
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
	    inputTokens: number;
	    outputTokens: number;
	    turns: number;
	    documents: HostedArtefactView[];
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
	        this.inputTokens = source["inputTokens"];
	        this.outputTokens = source["outputTokens"];
	        this.turns = source["turns"];
	        this.documents = this.convertValues(source["documents"], HostedArtefactView);
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
	
	
	export class OutputFileView {
	    path: string;
	    bytes: number;
	    previewKind: string;
	    preview?: string;
	    noPreview?: string;
	
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
	export class RunDefaultsView {
	    timeoutSeconds: number;
	    maxTurns: number;
	    policyPath: string;
	    where: string;
	    outputDir?: string;
	
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
	
	    static createFrom(source: any = {}) {
	        return new SettingsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultRegistry = source["defaultRegistry"];
	        this.registries = this.convertValues(source["registries"], RegistryView);
	        this.configPath = source["configPath"];
	        this.onboarded = source["onboarded"];
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
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.tools = source["tools"];
	        this.env = this.convertValues(source["env"], EnvVar);
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
	    summary: string;
	    fixedIn: string[];
	
	    static createFrom(source: any = {}) {
	        return new Advisory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.severity = source["severity"];
	        this.summary = source["summary"];
	        this.fixedIn = source["fixedIn"];
	    }
	}
	export class Status {
	    ref: string;
	    status: string;
	    maxSeverity?: string;
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

