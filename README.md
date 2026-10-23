# bb-mask-sdk

Data Mask anonimleştirme API’si için istemci SDK’ları. Paketler model çağırmaz; `anonymize` ve `restore` servise gider.

| Dil | Klasör | Paket | Alt sınır | Durum |
|---|---|---|---|---|
| Python | [`python/`](python/) | PyPI `bb-mask`, import `bb_mask` | 3.11+ | hazır |
| Node | [`node/`](node/) | npm `bb-mask` | 18+ | hazır |
| Go | [`go/`](go/) | modül `github.com/bigbrains/bb-mask-sdk/go`, paket `bbmask` | 1.22+ | hazır |
| C# | [`csharp/`](csharp/) | NuGet `bb-mask`, ad alanı `BigBrains` | .NET 8+ | hazır |
| Java | [`java/`](java/) | Maven `com.bigbrains:bb-mask`, paket `com.bigbrains.bbmask` | Java 17+ | hazır |

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

Modül yolu `github.com/bigbrains/bb-mask-sdk/go`, paket adı `bbmask`. Go 1.22+. Yayın öncesi:

```bash
go test ./...
```

Komut `go/` klasöründen çalışır.

Kullanım `go/README.md` içindedir. `go.mod` ilk satırı, paketin yayınlanacağı depo adresiyle aynı olmalıdır.

## C#

.NET 8. NuGet adı `bb-mask`. Yayın öncesi:

```bash
dotnet test BbMask.Tests/BbMask.Tests.csproj
```

Komut `csharp/` klasöründen çalışır. Kullanım `csharp/README.md` içindedir.

## Java

Java 17. Maven koordinatı `com.bigbrains:bb-mask`. Yayın öncesi:

```bash
mvn test
```

Komut `java/` klasöründen çalışır. Kullanım `java/README.md` içindedir. `pom.xml` içindeki `groupId`, paketin yayınlanacağı kuruluşa göre değişir.
