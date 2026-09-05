# canary, both halves in one recording — render with:  vhs halves.tape
# produces assets/halves.gif
#
# canary is two birds from one binary and the README says so twice. This is the
# only asset that shows it: the same command, above a shell prompt and then in
# the shape Claude Code asks for.
#
# HOME is a throwaway directory, so the recording carries no real history, no
# real session and none of the debt line from the machine doing the recording.
# The session JSON is written on camera, from the fields Claude Code actually
# sends: a transcript path and the session duration.
#
# The `; echo` on the second command is not decoration: canary's last status
# row ends without a newline so that whatever shares the row can sit beside it,
# and in a plain terminal the shell prompt would otherwise land inside the
# phrase.
#
# Every Type line is a backtick string. vhs cannot parse an escaped double
# quote inside a "..." string.

Output assets/halves.gif

Set Shell "bash"
Set FontSize 26
Set Width 1200
Set Height 620
Set Padding 24

Hide
Type `export HOME=$(mktemp -d /tmp/cXX) && cd "$HOME" && export PATH=/opt/homebrew/bin:$PATH`
Enter
Type `python3 -c "import json;c=lambda o:json.dumps(o,separators=(chr(44),chr(58)));u=c({'type':'user','message':{'role':'user','content':'make the retry loop stop after three'}});a=c({'type':'assistant','message':{'role':'assistant','content':[{'type':'tool_use','name':'Edit','input':{'file_path':'/src/retry.go'}}]}});e=c({'type':'user','message':{'content':[{'type':'tool_result','is_error':True,'content':'exit status 1'}]}});open('t.jsonl','w').write(chr(10).join([u,a]*24+[e]*4))"`
Enter
Type `printf '{\"transcript_path\":\"%s/t.jsonl\",\"cost\":{\"total_duration_ms\":14400000}}' "$HOME" > session.json`
Enter
Type `export PS1='$ ' && eval "$(canary init bash)" && clear`
Enter
Sleep 2s
Show

Type `# one bird above your shell prompt`
Enter
Sleep 1500ms
Type `ls`
Enter
Sleep 3s

Hide
# The prompt bird is switched off for the second half: Claude Code's status
# line prints its last row without a trailing newline, so that whatever shares
# the row can sit beside it, and a prompt bird redrawing underneath lands on
# top of the phrase. Two birds fighting for one row is a recording artefact,
# not what either half does on its own.
Type `PROMPT_COMMAND= && trap - DEBUG && clear`
Enter
Sleep 1s
Show

Type `# one inside Claude Code, from the same binary`
Enter
Sleep 1500ms
Type `canary statusline < session.json; echo`
Enter
Sleep 5s
