# Usage

hax is a terminal application. Start it with:

```bash
hax
```

## First run

On first run, hax asks you to pick a network (mainnet or testnet4) and a backend (Electrum or Bitcoin Core RPC). Then you can create or restore a wallet:

1. Create a wallet, write the seed phrase down on paper. Never store it digitally.
2. Choose an address type. Native SegWit is the default.
3. Pick mainnet or testnet4. For testnet4 coins, use a faucet such as [mempool.space/testnet4/faucet](https://mempool.space/testnet4/faucet).
4. Use `r` to receive, `s` to send, `h` for history, `u` for UTXOs, `t` for settings, and `?` for all key bindings.

Try testnet4 first.

## Key bindings

| Key | Screen |
|---|---|
| `r` | Receive |
| `s` | Send |
| `h` | History |
| `u` | UTXOs |
| `t` | Settings |
| `?` | Help and all key bindings |

## Backends

hax can use an Electrum server over TLS, with optional Tor SOCKS5, or your own Bitcoin Core node over JSON-RPC. Full history from Bitcoin Core needs `-blockfilterindex=1`.

You choose the backend on first run and can change it in Settings.

## Configuration

All state lives in `~/.hax/`:

| Path | Contents |
|---|---|
| `~/.hax/<name>.wallet` | Encrypted wallet (seed, optional passphrase, accounts, labels, cache) |
| `~/.hax/known_servers.json` | Trusted Electrum server keys |
| `~/.hax/config.json` | Backend selection, display unit, gap limit, recent servers |
| `~/.hax/*_labels_bip329.jsonl`, `~/.hax/*_koinly.csv` | Exports (`0600`) |

## Debug logging

Set `DEBUG=1` to write a debug log. Never share debug logs publicly without reviewing them first.

Wallet storage and encryption details are in [security-model.md](security-model.md). USB hardware wallet setup is covered in [hardware-wallet-compatibility.md](hardware-wallet-compatibility.md).
