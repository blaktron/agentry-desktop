// Agents: the persistent agents this app offers, each with its Enable
// switch, Start or Stop, Configure and Reset. The sidebar lists the enabled
// ones beneath Agents, each opening its own page.
import { PageHeader } from "../ui";
import PersistentAgentsSettings from "./SettingsPersistentAgents";

export default function Agents({ registry, onOpenAgent }: { registry: string; onOpenAgent: (name: string) => void }) {
  return (
    <div data-page="agents">
      <PageHeader
        title="Agents"
        sub="Turn your persistent agents on, start and stop them, and configure what each is given."
      />
      <PersistentAgentsSettings registry={registry} onOpenAgent={onOpenAgent} />
    </div>
  );
}
