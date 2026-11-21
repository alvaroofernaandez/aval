---
description: Close the working day: save decisions to Engram, session summary, pending list
---
Close today's session:

1. Save to Engram (`mem_save`, `capture_prompt: false`) every decision,
   bugfix or discovery from this session not saved yet. Resolve conflicts
   with one `mem_judge_many`.
2. Call `mem_session_summary` with goal, discoveries, accomplished, next steps
   and relevant files.
3. Update the `odd/tasks/*.md` checkboxes for the work done, if a plan exists.
4. Reply in the user's language with: what was done today, what is pending
   (ordered), and anything left uncommitted or unpushed in the repos touched.

Do not start new work, commit or push.
