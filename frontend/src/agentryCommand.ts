// What Settings and the first-run wizard say about the agentry command a
// macOS bundle carries (pages/AgentryCommand.tsx), apart from the page so it
// is tested on its own.

// CommandView is the part of app.CLICommandView the lines read.
export type CommandView = {
  version?: string;
  status: {
    path: string;
    linked: boolean;
    other: boolean;
    installable: boolean;
    resolves?: string;
    resolvesTo?: string;
  };
};

// commandLines is what the card says about the command, in order.
export function commandLines(v: CommandView): string[] {
  const s = v.status;
  const version = v.version ? `agentry ${v.version}` : "the agentry command";
  const lines: string[] = [];
  if (s.linked) {
    lines.push(`Installed: ${s.path} links to ${version}, which this app carries, so it updates when the app does.`);
  } else if (s.other) {
    lines.push(`${s.path} is an agentry this app did not put there, which it leaves as it is.`);
  } else {
    lines.push(`Not installed. Install puts ${version}, which this app carries, on your PATH as a link at ${s.path}, so it updates when the app does.`);
  }
  if (!s.installable) {
    lines.push("Move Agentry Desktop to your Applications folder and open it from there to install the command.");
  }
  if (s.resolves) {
    lines.push(`In a new terminal, agentry is ${s.resolves}${s.resolvesTo ? ` (${s.resolvesTo})` : ""}.`);
  } else {
    lines.push("A new terminal finds no agentry command.");
  }
  return lines;
}
