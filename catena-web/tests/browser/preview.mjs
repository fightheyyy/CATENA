import { fixtureServer } from "./fixture.mjs";
import { populateWorkspace } from "./workspace-sample.mjs";

const fixture = await fixtureServer();
populateWorkspace(fixture.state);
console.log(`Catena design preview — synthetic local data: ${fixture.origin}`);
process.on("SIGINT", async () => { await fixture.close(); process.exit(0); });
