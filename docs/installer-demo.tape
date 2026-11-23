# VHS tape for the installer demo GIF.
#
#   vhs docs/installer-demo.tape
#
# Records a real install into a throwaway HOME, so the recording shows actual
# "linked" results rather than a dry run's "would install", and can be remade
# at any time without touching a real machine.
#
# Typing speed is left close to human. A demo sped up until the keystrokes blur
# stops being a demonstration and becomes decoration.

Output docs/assets/installer.gif

Set Shell "bash"
Set FontSize 18
Set Width 1200
Set Height 760
Set Padding 24
Set Framerate 24

# A neutral black terminal with a plain ANSI palette: aval paints no canvas, so
# the recording shows the default `aval` theme as it looks on any dark terminal,
# ink on black with colour only on the state glyphs.
Set Theme { "name": "Black", "background": "#000000", "foreground": "#ffffff", "black": "#000000", "red": "#e46876", "green": "#98bb6c", "yellow": "#e6c384", "blue": "#7e9cd8", "magenta": "#957fb8", "cyan": "#7aa89f", "white": "#c8c8c8", "brightBlack": "#727272", "brightRed": "#e46876", "brightGreen": "#98bb6c", "brightYellow": "#e6c384", "brightBlue": "#7e9cd8", "brightMagenta": "#957fb8", "brightCyan": "#7aa89f", "brightWhite": "#ffffff" }

Hide
# A throwaway HOME so the install is real but harmless: the recording shows
# genuine "linked" results and cannot touch the machine it runs on.
#
# The binary is invoked as `aval`, which is what someone who
# installed it globally types. npx is not used here because its package cache
# lives in the real HOME, and replacing HOME turns the first frame into an
# "Ok to proceed?" prompt.
Type "export AVAL_NO_UPDATE_CHECK=1 AVAL_REPO=$PWD" Enter
Type "D=$(mktemp -d) && go build -o $D/aval ./cmd/aval && export PATH=$D:$PATH HOME=$(mktemp -d)" Enter
Type "clear" Enter
Show

Type "aval install"
Sleep 700ms
Enter

# The select screen: every group says what will happen to its files (new,
# linked, replaces) before anything is written. Let it land before moving.
Sleep 2500ms

# Move through the groups so the preview counts under each are readable.
Down
Sleep 1s
Down
Sleep 1s

# Open the details of one group (right arrow), then close them again.
Right
Sleep 2500ms
Left
Sleep 800ms

# Deselect one, so the choice reads as real and the footer counts respond.
Space
Sleep 1600ms
Space
Sleep 1200ms

# Install: inline progress, one line per step.
Enter
Sleep 5s

# Hold on the done screen: counts, the backup path, the undo command and the
# next steps are the part worth reading.
Sleep 5s
