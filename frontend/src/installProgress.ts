// The stage an install is at, as Go reports it (install:progress): the
// button that started the install says it in place of "Installing…".
import { useEffect, useState } from "react";
import { EventsOn } from "./host";

/** The stage of the install under way while active, "" between stages and
 *  once it is over; the button's own word stands in for "". */
export function useInstallStage(active: boolean): string {
  const [stage, setStage] = useState("");
  useEffect(() => {
    if (!active) {
      setStage("");
      return;
    }
    return EventsOn("install:progress", (s: string) => setStage(typeof s === "string" ? s : ""));
  }, [active]);
  return active ? stage : "";
}
