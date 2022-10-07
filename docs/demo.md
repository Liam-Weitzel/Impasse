# Demo Server

We have a demo server at `slt-play.intevation.de` running on port `2222`. 
You can connect it **without any passphrase or key**:
```
ssh -p 2222 slt-play.intevation.de
```
The fingerpint is `SHA256:PDwMjwl8UUFz09LszWaBpNKhCheuTXojbwj0a21cXCI`.
You can suppress the check by adding `-o "StrictHostKeyChecking=no"`
to the call above.

As this server runs on hardware with limited ressources we are restricting
the access a bit:
- The server accepts only 30 users being logged in at once.
- You will be kicked from the server if you do nothing for 90 seconds.

## YMMV ... choose a fast terminal emulator

You need a fast terminal emulator to have a good user experience.
[kitty](https://sw.kovidgoyal.net/kitty/) is a good choice.

If you use a VTE-based terminal like the [GNOME terminal](https://wiki.gnome.org/Apps/Terminal) or
the [XFCE4 terminal](https://docs.xfce.org/apps/terminal/start) we  encourage  you
to set the env var `COLORTERM=truecolor` and add the option `'SendEnv COLORTERM'` to
the `ssh` call:
```
COLORTERM=truecolor ssh -p 2222 slt-play.intevation.de -o 'SendEnv COLORTERM'
```
This is also recommended if you use [Konsole](https://konsole.kde.org/) KDE's terminal emulator,
[QTerminal](https://github.com/lxqt/qterminal) or [Alacritty](https://alacritty.org/).

It is reported that the usage of terminals from the  [RXVT family](https://rxvt.sourceforge.net/)
do not lead to good results.

