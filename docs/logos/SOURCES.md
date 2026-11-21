# Logo sources

Each tool's own mark, taken from the project itself. They are trademarks of
their owners, shown only to name the tools this setup uses. PNGs are resized
to at most 192 px so the generated SVGs stay small.

`docs/brand.py` embeds these into `docs/assets/stack-*.svg`.

| File | Source |
| :--- | :--- |
| `apple.svg` | devicons/devicon — `icons/apple/apple-original.svg` |
| `bash.svg` | devicons/devicon — `icons/bash/bash-original.svg` |
| `bun.svg` | devicons/devicon — `icons/bun/bun-original.svg` |
| `claude.svg` | devicons/devicon — `icons/claude/claude-original.svg` |
| `docker.svg` | devicons/devicon — `icons/docker/docker-original.svg` |
| `git.svg` | devicons/devicon — `icons/git/git-original.svg` |
| `github.svg` | devicons/devicon — `icons/github/github-original.svg` |
| `go.svg` | devicons/devicon — `icons/go/go-original.svg` |
| `homebrew.svg` | devicons/devicon — `icons/homebrew/homebrew-original.svg` |
| `linux.svg` | devicons/devicon — `icons/linux/linux-original.svg` |
| `neovim.svg` | devicons/devicon — `icons/neovim/neovim-original.svg` |
| `nodejs.svg` | devicons/devicon — `icons/nodejs/nodejs-original.svg` |
| `npm.svg` | devicons/devicon — `icons/npm/npm-original.svg` |
| `python.svg` | devicons/devicon — `icons/python/python-original.svg` |
| `tmux.svg` | devicons/devicon — `icons/tmux/tmux-original.svg` |
| `windows.svg` | devicons/devicon — `icons/windows11/windows11-original.svg` |
| `zsh.svg` | devicons/devicon — `icons/zsh/zsh-original.svg` |
| `ghostty.png` | ghostty-org/ghostty — `images/icons/icon_256.png` |
| `opencode.svg` | anomalyco/opencode — `packages/ui/src/assets/favicon/favicon-v3.svg` |
| `yazi.png` | sxyazi/yazi — `assets/logo.png` |
| `herdr.svg` | herdrdev/herdr — `assets/logo.svg` |
| `engram.png` | Gentleman-Programming/engram — `assets/branding/engram-logo-only.png` |
| `rtk.png` | rtk-ai — GitHub organization avatar |
| `gstack.png` | garrytan/gstack — `extension/icons/icon-128.png` |
| `kiri.svg` | libracoder/kiri — `brand/logo/kiri-favicon.svg` |
| `lightpanda.png` | lightpanda-io/browser — README logo, `cdn.lightpanda.io/assets/images/logo/lpd-logo.png` |
| `gitleaks.png` | gitleaks/gitleaks — README logo image |
| `fd.svg` | sharkdp/fd — `doc/logo.svg` |
| `bat.png` | sharkdp/bat — `doc/logo-header.svg`, cropped to the wings mark, grey header band made transparent |
| `bat-dark.png` | `bat.png` lightened for the dark theme (ImageMagick `-modulate 175,150,100`): the navy mark measured 2% legible on `#262626` |
| `eza.png` | eza-community — GitHub organization avatar |
| `fzf.png` | junegunn/i — `fzf.png`, cropped to the mark |
| `zoxide.svg` | ajeetdsouza/zoxide — `contrib/logo-light.svg` |
| `atuin.png` | atuinsh — GitHub organization avatar, white background made transparent (flood fill from the corners) |

**No published logo:** ripgrep (BurntSushi/ripgrep has no image assets) and
delta (dandavison/delta; its docs favicon is mdBook's default). Both get a
monogram tile rather than an invented mark.

On the dark theme, `docs/brand.py` lightens the dark tones of apple, github,
zsh, bash and tmux (brand colours are kept), because those marks are drawn
black and disappear on a dark card. The files here stay untouched.
