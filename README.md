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

Subir una etiqueta `vX.Y.Z`; GitHub Actions fabrica las dos apps y crea el release.

```sh
git tag v1.3.0 && git push origin v1.3.0
```

Fabricar a mano: `./build.sh 1.3.0 joshen-am/idle` (deja los zip en `dist/`).
