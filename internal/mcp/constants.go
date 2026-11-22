package mcp

const requestSchema = `{
  "type":"object",
  "properties":{
    "id":{"type":"string","description":"Unique stable job ID; reuse on a retry to avoid duplicate execution."},
    "command":{"type":"array","items":{"type":"string"},"minItems":1,"description":"Executable and arguments, without shell expansion. Invoke a shell explicitly for scripts."},
    "directory":{"type":"string","description":"Absolute working directory."},
    "environment":{"type":"object","additionalProperties":{"type":"string"},"description":"Environment overrides; inherited values remain available."},
    "timeout_seconds":{"type":"integer","minimum":0,"maximum":31536000,"default":0,"description":"Explicit deadline; kills the process tree. Zero disables."},
    "stall_seconds":{"type":"integer","minimum":0,"maximum":31536000,"default":300,"description":"Warn after this period with no observed process-tree activity. Zero disables. Never kills the job."},
    "heartbeat":{"type":"string","description":"Optional absolute path to an application-maintained progress file."},
    "wait_for_children":{"type":"boolean","default":false,"description":"By default the foreground command's exit completes the job and remaining owned children are terminated. Set true only when the command intentionally delegates work and exits before its children finish."}
  },
  "required":["id","command","directory"],
  "additionalProperties":false
}`

const idSchema = `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"],"additionalProperties":false}`

const waitSchema = `{"type":"object","properties":{"id":{"type":"string"},"after_warning":{"type":"boolean","default":false,"description":"Acknowledge the current warning and keep waiting for completion or a fresh stall episode."}},"required":["id"],"additionalProperties":false}`

const instructions = "Use run for long commands when there is no independent work, or start once, do other work, then wait once. Wait blocks until completion or a suspected stall: do not loop over inspect, sleep, log reads or CPU checks. Internal monitoring consumes no model calls. Reuse stable job IDs after uncertain submission. Cancelling a tool request only detaches the wait; cancel explicitly terminates the job tree. A suspected stall is evidence to investigate, not proof of deadlock. Set Codex tool_timeout_sec above the longest expected wait."
