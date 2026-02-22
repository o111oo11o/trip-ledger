# /ship — Simplify → Test → Deploy → Document

Run a full ship cycle:
1. Simplify the codebase
2. Test and fix issues
3. Loop steps 1–2 until tests pass and no new simplifications are found
4. Deploy
5. Update CLAUDE.md

---

Execute the following workflow. Do not ask for confirmation between steps — proceed autonomously.

## Loop phase (repeat until exit condition)

**Step A — Simplify**
Invoke the `simplifier` subagent. Wait for it to finish and capture its report.

**Step B — Test**
Invoke the `tester` subagent. Wait for it to finish and capture its report.

**Exit condition check:**
- If the tester ends with `TESTS_FAIL`: stop the loop, report the failures, and do NOT deploy. Ask the user to review.
- If the simplifier made zero changes AND the tester ends with `TESTS_PASS`: exit the loop.
- If the simplifier made changes and the tester passes: run the loop again (back to Step A) to check if the simplifier finds anything new in the now-fixed code.
- Maximum 4 loop iterations. If the loop hasn't converged by iteration 4, exit anyway and proceed to deploy if tests pass.

## Post-loop phase (only if tests pass)

**Step C — Deploy**
Invoke the `deployer` subagent.

**Step D — Document**
Invoke the `doc-updater` subagent. Pass it the simplification report, test report, and deploy report as context.

## Final summary

Print a summary of all four reports and the total number of loop iterations.
