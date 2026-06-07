Output assets/demo.gif
Set Shell "bash"
Set FontSize 18
Set Width 1200
Set Height 760
Set Theme "Dracula"

Type "./bin/gh-hound runs --no-tui --json --fake-scenario failure | jq '{repo, branch, failure: .runs[0].failed[0]}'"
Enter
Sleep 2s

Type "./bin/gh-hound watch --json --fake-scenario failure | jq '.runs[0].failed[0].step'"
Enter
Sleep 1s
