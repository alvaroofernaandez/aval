package i18n

// spanishCLI holds the keys of the human CLI output (cmd/aval, internal/ui/cli).
var spanishCLI = catalog{
	// usage
	"cli.usage.tagline":      {other: "Nada pasa sin evidencia."},
	"cli.usage.about":        {other: "aval instala y mantiene los dotfiles y las configuraciones de agentes de este equipo."},
	"cli.usage.heading":      {other: "Uso:"},
	"cli.usage.invocation":   {other: "aval <comando> [opciones]"},
	"cli.usage.group.setup":  {other: "Preparación"},
	"cli.usage.group.health": {other: "Salud"},
	"cli.usage.group.undo":   {other: "Deshacer"},
	"cli.usage.group.agent":  {other: "Herramientas de agentes"},
	"cli.usage.group.plumb":  {other: "Piezas internas"},
	"cli.usage.plumbing":     {other: "Las llaman los hooks y los scripts, pocas veces a mano."},

	"cli.usage.cmd.install":    {other: "enlaza las configuraciones del repositorio en HOME (TUI; --yes para omitirla)"},
	"cli.usage.cmd.update":     {other: "actualiza el repositorio y vuelve a aplicar"},
	"cli.usage.cmd.doctor":     {other: "comprueba este equipo frente al repositorio"},
	"cli.usage.cmd.sync":       {other: "fusiona los ajustes del repositorio en la configuración activa"},
	"cli.usage.cmd.rules":      {other: "refleja las reglas compartidas de CLAUDE.md en el AGENTS.md de OpenCode"},
	"cli.usage.cmd.budget":     {other: "contexto fijo por agente y latencia de los hooks"},
	"cli.usage.cmd.review":     {other: "nivel de riesgo según git y lentes de revisión"},
	"cli.usage.cmd.design":     {other: "controles de diseño para el proyecto del directorio actual"},
	"cli.usage.cmd.config":     {other: "muestra o cambia los ajustes de aval (tema, glifos, movimiento)"},
	"cli.usage.cmd.restore":    {other: "lista o restaura copias de seguridad"},
	"cli.usage.cmd.uninstall":  {other: "quita los enlaces de aval y restaura lo que desplazaron"},
	"cli.usage.cmd.dev":        {other: "HOME aislado para desarrollar aval"},
	"cli.usage.cmd.hook":       {other: "hooks de agentes y el control previo al push de git"},
	"cli.usage.cmd.statusline": {other: "línea de estado de Claude Code"},
	"cli.usage.cmd.tmux":       {other: "utilidades de tmux, Ghostty y zsh"},
	"cli.usage.cmd.macos":      {other: "ajustes de macOS y LaunchAgents"},
	"cli.usage.cmd.reap":       {other: "termina los procesos de pruebas huérfanos"},
	"cli.usage.cmd.kiri":       {other: "deja Docker listo en Kiri"},
	"cli.usage.cmd.docker":     {other: "limpiezas seguras de Docker en el contexto kiri"},
	"cli.usage.cmd.deck":       {other: "convierte un plan en una presentación PDF, generada en local"},
	"cli.usage.cmd.pdf":        {other: "crea proyectos de informes PDF y sus recursos de marca"},
	"cli.usage.cmd.release":    {other: "publica una versión"},
	"cli.usage.cmd.version":    {other: "versión, commit, fecha de compilación y canal"},
	"cli.usage.cmd.help":       {other: "muestra esta ayuda"},

	"cli.usage.flags":      {other: "Opciones globales:"},
	"cli.usage.flag.repo":  {other: "el repositorio de aval que se usa"},
	"cli.usage.flag.theme": {other: "tema de color para esta ejecución (aval, terminal, mono, catppuccin, ...)"},
	"cli.usage.flag.plain": {other: "monocromo, glifos ASCII y sin movimiento"},
	"cli.usage.flag.lang":  {other: "idioma de la salida para personas: auto, en o es"},
	"cli.usage.settings": {other: "Ajustes: theme, background, glyphs, motion, update_check, language. La precedencia\n" +
		"es opción, luego variable AVAL_*, luego ~/.config/aval/config; language sigue\n" +
		"después la configuración regional (LC_ALL, LC_MESSAGES, LANG). NO_COLOR desactiva el color."},
	"cli.usage.repo": {other: "El repositorio se busca, en orden, en: --repo, $AVAL_REPO, la ubicación que\n" +
		"'aval install' guardó en ~/.config/aval/config y el directorio actual.\n" +
		"La primera vez: clone el repositorio, entre en él y ejecute 'aval install'."},
	"cli.usage.first_run": {other: "Aún sin configurar. Ejecute: aval install"},
}
