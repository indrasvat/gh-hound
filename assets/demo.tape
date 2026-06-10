Output assets/demo.gif

Require bash

Set Shell "bash"
Set FontSize 16
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set Width 1280
Set Height 744
Set Padding 0
Set Framerate 12
Set PlaybackSpeed 1.0
Set Theme "Dracula"
Set TypingSpeed 12ms

Hide
Type "export TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1 HOUND_WELCOME=false; clear; ./bin/gh-hound --version"
Enter
Sleep 500ms
Show
Sleep 2s

Hide
Type "clear"
Enter
Show

Type "./bin/gh-hound -R indrasvat/gh-hound -A"
Enter
Sleep 4s

Down 2
Sleep 600ms
Enter
Sleep 3s

Type "l"
Sleep 4s

Type "/"
Type "go"
Enter
Sleep 2s

Escape
Sleep 800ms
Type "?"
Sleep 2s

Escape
Sleep 600ms
Type "r"
Sleep 2s

Escape
Sleep 500ms
Ctrl+C
