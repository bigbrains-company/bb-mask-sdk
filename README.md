# bb-mask-sdk

Data Mask anonimleştirme API’si için istemci SDK’ları. Paketler model çağırmaz; `anonymize` ve `restore` servise gider.

| Dil | Klasör | Paket | Durum |
|---|---|---|---|
| Python | [`python/`](python/) | PyPI `bb-mask`, import `bb_mask` | hazır |
| Node | [`node/`](node/) | npm `bb-mask` | hazır |
| Go | [`go/`](go/) | | henüz yok |
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
