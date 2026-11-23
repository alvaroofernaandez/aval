package i18n

// spanish covers every English key, in neutral, professional Spanish, in the
// same parts as English.
var spanish = join(spanishCore, spanishUI, spanishCLI)

var spanishCore = catalog{
	"config.invalid_language": {other: "language = {value} no es un idioma válido. Valores válidos: {valid}."},
	"config.did_you_mean":     {other: "¿Quiso decir {suggestion}?"},
}
