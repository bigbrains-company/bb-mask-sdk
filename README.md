# bb-mask-sdk

Data Mask anonimleştirme API’si için istemci SDK’ları. Paketler model çağırmaz; `anonymize` ve `restore` servise gider.

| Dil | Klasör | Paket | Durum |
|---|---|---|---|
| Python | [`python/`](python/) | PyPI `bb-mask`, import `bb_mask` | hazır |
| Node | [`node/`](node/) | npm `bb-mask` | hazır |
| Go | [`go/`](go/) | modül `github.com/bigbrains/bb-mask-sdk/go`, paket `bbmask` | hazır |
| C# | [`csharp/`](csharp/) | | henüz yok |
| Java | [`java/`](java/) | | henüz yok |

Lisans: MIT.

## Python

```bash
pip install bb-mask
```

Bu depoyu çektikten sonra:

```bash
pip install -e python
```

Kullanım `python/README.md` içindedir.

## Node

Paket yayınlanınca `npm install bb-mask`. Bu depoyu çektikten sonra:

```bash
npm install ./node
```

Kullanım `node/README.md` içindedir. Test: `npm test --prefix node`.

## Go

Modül yolu `github.com/bigbrains/bb-mask-sdk/go`, paket adı `bbmask`. Yayın öncesi:

```bash
go test ./...
```

Komut `go/` klasöründen çalışır.

Kullanım `go/README.md` içindedir. `go.mod` ilk satırı, paketin yayınlanacağı depo adresiyle aynı olmalıdır.
