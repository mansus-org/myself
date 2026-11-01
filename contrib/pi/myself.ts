/**
 * pi extension: adds `myself prime` to the context on the first agent run of a
 * session, like contrib/hooks/session-start.sh does for Claude Code.
 *
 * Does nothing if myself is missing, fails or takes longer than 10s.
 *
 * Environment:
 *   MYSELF_BIN           myself binary (default: myself on PATH)
 *   MYSELF_PRIME_BUDGET  characters passed to `myself prime --budget` (default 4000)
 *   MYSELF_FILE, MYSELF_URL, MYSELF_TOKEN  passed through to myself
 */

import { execFile } from "node:child_process";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

function prime(): Promise<string> {
	const bin = process.env.MYSELF_BIN || "myself";
	const budget = process.env.MYSELF_PRIME_BUDGET || "4000";
	return new Promise((resolve) => {
		execFile(bin, ["prime", "--budget", budget], { timeout: 10_000, encoding: "utf8" }, (err, stdout) =>
			resolve(err ? "" : stdout.trim()),
		);
	});
}

export default function myself(pi: ExtensionAPI) {
	let primed = false;

	pi.on("session_start", () => {
		primed = false;
	});

	pi.on("before_agent_start", async () => {
		if (primed) return;
		primed = true;
		const text = await prime();
		if (!text) return;
		return {
			message: {
				customType: "myself-prime",
				content: `What myself remembers (\`myself prime\`; query more with myself describe|search|path|log):\n${text}`,
				display: false,
			},
		};
	});
}
