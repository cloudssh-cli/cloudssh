# Records docs/demo.gif from the built-in demo data (no cloud account, no network).
# Run with: make demo-gif   (VHS runs in Docker; nothing to install)
Output docs/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1280
Set Height 720
Set Padding 12
Set TypingSpeed 90ms
Set Theme "Catppuccin Mocha"

# Setup (hidden): the Linux demo binary, and a throwaway HOME with an empty key file
Hide
Type "export PATH=/vhs/bin/linux:$PATH TERM=xterm-256color HOME=/home/demo" Enter
Type "mkdir -p $HOME/.cloudssh/pems && touch $HOME/.cloudssh/pems/prod-key.pem && chmod 600 $HOME/.cloudssh/pems/prod-key.pem && clear" Enter
Show

Type "cloudssh --demo" Sleep 500ms Enter
Sleep 4s

# search
Type "/"
Sleep 400ms
Type "web"
Sleep 1500ms
Enter
Sleep 2s
Escape
Sleep 800ms

# auto scaling groups -> drill into one
Type "4"
Sleep 1800ms
Down
Sleep 700ms
Enter
Sleep 2500ms

# ssh dialog: switch to EC2 Instance Connect
Type "S"
Sleep 2s
Tab
Sleep 600ms
Right
Sleep 2500ms
Escape
Sleep 800ms

# dashboard and help
Type "d"
Sleep 3s
Type "?"
Sleep 3s
Escape
Sleep 500ms
Type "q"
Sleep 800ms
