# Cómo publicar una versión

aval usa [SemVer](https://semver.org): `vX.Y.Z` para estables y `vX.Y.Z-rc.N` / `vX.Y.Z-beta.N`
para prereleases. Una versión es un **tag anotado** en `main`; al empujarlo, `release.yml` hace
el resto. El historial resumido está en [CHANGELOG.md](../CHANGELOG.md); desde la primera versión,
las notas salen de la release de GitHub.

## El flujo (3 comandos)

```sh
aval release status    # último tag, commits sin publicar, versión sugerida
aval release rc        # tag v0.2.0-rc.1 (la siguiente rc suma 1)
aval release promote   # la rc se convierte en v0.2.0, sobre el mismo commit
```

Cada comando de corte (`rc`, `beta`, `patch`, `minor`, `major`, `promote`):

1. comprueba: árbol limpio, rama `main`, al día con `origin/main`, `go test ./...` y CI verde en
   `HEAD` (esto último solo si `gh` está disponible);
2. enseña la versión, los commits y las notas agrupadas (Breaking, Features, Fixes, Performance;
   `docs`, `test`, `ci` y `chore` no aparecen);
3. pregunta `y/N` (`--yes` lo salta; `--dry-run` solo enseña y no toca nada);
4. crea el tag anotado y lo empuja con `git push origin <tag>`;
5. imprime la URL del workflow: **falta aprobar el entorno `release`** (solo el dueño puede).

Atajos: `aval release patch|minor|major` publica una estable sin pasar por rc.

### Qué versión sale

Antes de 1.0: `feat` y cambio incompatible (`!` o `BREAKING CHANGE:`) suben el minor;
`fix` y `perf`, el patch. Desde 1.0 es SemVer estándar. `docs`, `test`, `ci`, `chore` no
justifican versión: si solo hay eso, `aval release` dice que no hay nada que publicar.

`promote` solo etiqueta el commit de la rc, así que el binario estable es el que ya probaste.
Si `main` avanzó desde la rc, se niega: saca otra rc.

## Qué hace cada job

| Job | Permisos | Comprueba / hace |
| :--- | :--- | :--- |
| `preflight` | solo lectura | el tag tiene forma `vX.Y.Z[-rc.N\|-beta.N]`, es anotado y está en `origin/main`; `ci.yml` terminó en éxito sobre ese commit; `goreleaser check` y un `release --snapshot` compilan los 6 destinos |
| `publish` | `contents: write`, `id-token`, `attestations`; **entorno `release`** | GoReleaser compila, crea la release (prerelease si lleva `-`) y sube archivos + `SHA256SUMS`; atestación de procedencia sobre cada archivo |
| `verify` | solo lectura | baja lo publicado: `sha256sum -c SHA256SUMS`, `gh attestation verify` de cada archivo, y que el binario imprima la versión del tag |
| `npm` | **entorno `release`**, `id-token` | reutiliza los mismos binarios ya verificados; publica con `--tag next` si la versión lleva `-`, `latest` si no |

Los commits en `main` solo ejecutan `ci.yml`; nunca publican.

## Probar una rc

```sh
npx @alvaroofernaandez/aval@next version     # desde npm
gh release download v0.2.0-rc.1 -R alvaroofernaandez/aval --pattern '*linux_amd64*'
```

Los builds estables avisan de novedades en `latest`; los de prerelease, en `next` (y en
`latest`, para enterarse de que su estable salió). Un build de desarrollo (`dev`) nunca avisa.
`AVAL_NO_UPDATE_CHECK=1` lo desactiva.

## Verificar lo que descargas

```sh
gh release download v0.2.0 -R alvaroofernaandez/aval
sha256sum -c SHA256SUMS --ignore-missing          # macOS: shasum -a 256 -c SHA256SUMS --ignore-missing
gh attestation verify aval_0.2.0_linux_amd64.tar.gz --repo alvaroofernaandez/aval
tar -xzf aval_0.2.0_linux_amd64.tar.gz && ./aval version
```

## Si algo falla

- **El tag está mal y aún no se aprobó nada:** `git push origin :refs/tags/<tag>` y `git tag -d <tag>`.
- **Falló `verify` o `npm`:** vuelve a lanzar el workflow desde Actions (*Run workflow* con el tag);
  la subida de assets reemplaza los existentes y `npm` se salta lo ya publicado.
- **Subir GoReleaser:** cambia `GORELEASER_VERSION` y `GORELEASER_SHA256` en `release.yml` (el hash
  está en el `checksums.txt` de su release, línea `goreleaser_Linux_x86_64.tar.gz`) y prueba con
  `goreleaser release --snapshot --clean --skip=publish`.

## Requisitos únicos

Entorno `release` con aprobación del dueño y el secreto `NPM_TOKEN` (token de automatización con
permiso de publicación en `@alvaroofernaandez`). El tag móvil `latest` y la release «Latest build»
anteriores a este sistema no se usan: bórralos cuando salga la primera versión.
