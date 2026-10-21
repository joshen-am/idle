# Idle

Mini ventana de escritorio para Mac y Windows.

## Instalar

Descarga la última versión en **Releases** (`Idle-Mac.zip` o `Idle-Windows.zip`).
Después se actualiza sola cada vez que se abre.

- **Mac:** descomprime y arrastra `Idle.app` a Aplicaciones. La primera vez:
  Ajustes del Sistema → Privacidad y seguridad → "Abrir de todas formas".
- **Windows:** descomprime y abre `Idle.exe`. Si aparece "Windows protegió su PC":
  Más información → Ejecutar de todas formas.

## Publicar una versión

En GitHub: Actions → release → Run workflow, con la versión (ej. 1.3.0). Fabrica las dos apps y crea el release.


Fabricar a mano: `./build.sh 1.3.0 joshen-am/idle` (deja los zip en `dist/`).
