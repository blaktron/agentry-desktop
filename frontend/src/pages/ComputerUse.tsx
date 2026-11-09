// Plugins → Your computer: the person's approved applications, computer
// use, the runs using the computer now and the runs that used it.
import { PageHeader } from "../ui";
import { YourComputerSettings } from "./SettingsYourComputer";

export default function ComputerUse() {
  return (
    <div data-page="computer">
      <PageHeader
        title="Your computer"
        sub="The applications on this computer agents may use, each approved by you for its MCP server, its windows or both; the library of popular applications; computer use and what is kept of it; the runs using your computer now with Pause and Stop; and the runs that used it."
      />
      <YourComputerSettings />
    </div>
  );
}
