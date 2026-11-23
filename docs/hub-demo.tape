# VHS tape for the hub demo GIF.
#
#   vhs docs/hub-demo.tape
#
# Opens the hub in a throwaway HOME after a real install, so the status panel
# shows real data (a backup, a doctor summary) and the recording cannot touch
# the machine it runs on.

Output docs/assets/hub.gif

Set Shell "bash"
Set FontSize 18
Set Width 1200
Set Height 760
Set Padding 24
Set Framerate 24

# A neutral black terminal with a plain ANSI palette, as in the installer demo.
Set Theme { "name": "Black", "background": "#000000", "foreground": "#ffffff", "black": "#000000", "red": "#e46876", "green": "#98bb6c", "yellow": "#e6c384", "blue": "#7e9cd8", "magenta": "#957fb8", "cyan": "#7aa89f", "white": "#c8c8c8", "brightBlack": "#727272", "brightRed": "#e46876", "brightGreen": "#98bb6c", "brightYellow": "#e6c384", "brightBlue": "#7e9cd8", "brightMagenta": "#957fb8", "brightCyan": "#7aa89f", "brightWhite": "#ffffff" }

Hide
# Throwaway HOME, binary built from this checkout, no update check. The
# install runs hidden so the panel has real data when the hub opens.
Type "export AVAL_NO_UPDATE_CHECK=1 R=$PWD" Enter
Type "D=$(mktemp -d) && go build -o $D/aval ./cmd/aval && export PATH=$D:$PATH HOME=$(mktemp -d)" Enter
# The repo is reached through a link inside the throwaway HOME, so the panel
# shows ~/.aval instead of the absolute path of whoever records this.
Type "ln -s $R $HOME/.aval && export AVAL_REPO=$HOME/.aval" Enter
Type "aval install --yes > /dev/null 2>&1" Enter
Sleep 8s
Type "clear" Enter
Show

Type "aval"
Sleep 700ms
Enter
Sleep 3s

# Moving the cursor changes the detail pane beside the menu.
Down
Sleep 1500ms
Down
Sleep 1500ms
Down
Sleep 1500ms
Down
Sleep 1500ms

# A letter opens its entry directly: d is Doctor.
Type "d"
Sleep 3500ms
Escape
Sleep 1500ms

# Every key, then out.
Type "?"
Sleep 3s
Escape
Sleep 600ms
Type "q"
Sleep 1s
