# Deck fonts

The brand fonts of `aval deck`, embedded in the binary so a deck renders the same with no
network. They are the files the `google/fonts` repository ships, byte for byte: never subset,
converted or renamed, because IBM Plex reserves its name for unmodified files.

Taken from `google/fonts` at commit `06bfe0d62a98375cb09c942fa18e68bd32cff057` on 2026-11-23.

| File | Origin | Upstream version | SHA-256 |
| :-- | :-- | :-- | :-- |
| `SpaceGrotesk-wght.ttf` | `ofl/spacegrotesk/SpaceGrotesk[wght].ttf` | 2.000 (variable, wght 300-700) | `acad6de1fc93436f5c0f1f4137751ef04f1aea3063e7036535970ffcfbd79f72` |
| `IBMPlexSans-wdth-wght.ttf` | `ofl/ibmplexsans/IBMPlexSans[wdth,wght].ttf` | 3.201 (variable, wdth 75-100, wght 100-700) | `3b031aa4216174205bd8471f88a49b91f093169e9e87bd5262242bc5967fe2e3` |
| `OFL-spacegrotesk.txt` | `ofl/spacegrotesk/OFL.txt` | SIL Open Font License 1.1 | `564ce565c371c5e5bbf286006565a7c9aa55a9f56e7ca58d56e05d649dd61a72` |
| `OFL-ibmplexsans.txt` | `ofl/ibmplexsans/OFL.txt` | SIL Open Font License 1.1 | `7e6b2818edbd8f6a01ae80641cc8f16a51080d08fb4e532be3a0b6f74adb07da` |

Origin URL: `https://github.com/google/fonts/tree/06bfe0d62a98375cb09c942fa18e68bd32cff057/ofl`.
The licences also ship in every release archive (`.goreleaser.yaml`).
