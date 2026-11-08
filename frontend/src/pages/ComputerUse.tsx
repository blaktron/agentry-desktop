// Plugins → Computer use: the applications on this computer a QuickAgent may
// use, the runs using the computer now and the runs that used it.
import { PageHeader } from "../ui";
import { YourComputerSettings } from "./SettingsYourComputer";

export default function ComputerUse() {
  return (
    <div data-page="computer">
      <PageHeader
        title="Computer use"
        sub="The applications on this computer a QuickAgent may use through the MCP servers installed for them, each one's state here and what to do, its script screening, computer use and what is kept of it, the runs using your computer now with Pause and Stop, and the runs that used it."
      />
      <YourComputerSettings />
    </div>
  );
}
