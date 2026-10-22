# Idle

Mascota de escritorio para Mac y Windows: un personaje pixelado que vive en una esquina de la
pantalla, sin ícono, y con el que se puede jugar Pong (contra la CPU o en red).

- **Clic** en el personaje: menú (jugar, personaje, animaciones, sonido).
- **Arrastrarlo**: moverlo de lugar. Soltarlo en el **basurero** cierra Idle; para volver, se abre la app.
- **↑ y ↓ juntas, dos veces**: modo juego (las flechas son del juego) / modo trabajo.
- **Tienda**: cuerpos, ojos, colores y gorros que se compran con ¤, que se ganan jugando
  (más si le ganas a un CPU difícil o a un compañero).
- **Ranking**: victorias por juego tuyas, de tus compañeros (se pasan al jugar online) y de los CPU.
- **CPU**: TURBO, BIGOTES, KARL, BIT y MOCHI, cada uno con su forma de jugar y su personalidad.
  Si tu compañero no llega, uno de ellos ocupa su lugar. Cuando no estás, juegan entre ellos,
  se hacen amigos o enemigos y suben (despacito) en el ranking.
- **Diario**: todos los días a las 9 se abre solo y cuenta, con drama, lo que pasó ayer.

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
