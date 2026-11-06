# Getting started

What happens the first time you run `tele`, and what changes for you when you upgrade from 1.11.
Installing it is covered in the [README](../README.md#installation).

## First launch

```sh
tele
```

On first run, `tele` creates `~/.config/tele/config.yml` and shows a QR code.
Open Telegram on your phone, go to Settings > Devices > Link Desktop Device, and scan it.
If your account has a 2FA password, `tele` asks for it next.

The QR code needs a window of about 80 by 24.
In a smaller one, `tele` asks you to make the window larger.

To log in by phone number instead, press `p` or Enter.
`tele` then asks for:

- your phone number
- the code Telegram sends you
- your 2FA password, if you have one

Esc at any of these goes back to the QR code.

## Running as a service

Once you are logged in, `tele` asks once whether to keep running in the background as a service.
Say `y` and the account stays connected after you close the window, with notifications still arriving.
Say `n` or press Esc and `tele` runs only while a window is open.

You can change your mind later with `tele daemon install` or `tele daemon uninstall`.
`tele` does not ask where a service cannot be installed, or when one already is.
How the service runs on each system: [configuration.md](configuration.md#running-without-a-window).

## App key

`tele` reaches Telegram with an app key, which identifies the application rather than you.
Every build carries one, so logging in needs nothing from you.

If Telegram ever refuses the key, `tele` says `app key blocked by Telegram`.
This can happen to builds compiled from source, which all share one key.
[app-key.md](app-key.md) explains the two ways past it.

## Upgrading from 1.11

- Opening `tele` while another is running no longer fails: the second one joins the first as another window on the same account.
- After you log in, `tele` asks once about running as a service, as described above.
- Flags take two dashes now (`--config`, `--debug`); the short `-e` still works.
  The version and the theme tools became commands: `tele version`, `tele theme check`, `tele theme dump`.
- 1.11 and 2.0 cannot use the same account at the same time: quit 1.11 before starting 2.0.

Everything that changed: [CHANGELOG.md](../CHANGELOG.md)
