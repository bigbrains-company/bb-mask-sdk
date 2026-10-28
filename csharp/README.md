# bb-mask

Türkçe hukuki metindeki kişisel veriyi etiketler, model cevabındaki etiketleri geri açar. Paket ince bir HTTP istemcisidir. Model çağırmaz.

Müşteri Data Mask konsolundan `dm_test_` anahtarı alır. .NET veya ASP.NET Core servisi bu paketi kurar ve anahtarı `Init`'e verir. LLM satırı müşterinindir; SDK'nın öncesine `Anonymize`, sonrasına `Restore` konur. Paket framework'e bağlanmaz.

.NET 8+. Bağımlılık yok; `HttpClient` yeter. Python, Node, Go ve Java aynı deponun diğer klasörlerindedir.

```csharp
using BigBrains;

BbMask.Init(Environment.GetEnvironmentVariable("BB_ANONYM_TOKEN"));

var tagged = await BbMask.AnonymizeAsync("Davacı Hasan Demirkol, TC 62601815964");
var cevap = await kendiLlm(tagged.Text);
var duz = await BbMask.RestoreAsync(tagged.DocId, cevap);
await BbMask.DeleteAsync(tagged.DocId);
```

Her metodun `Async` eki olmayan senkron kardeşi vardır. ASP.NET Core içinde `Async` kullanın. Hatalar `BBError` fırlatır. `Message` ayrıntı metnidir.

| Python | C# |
|---|---|
| `init` | `Init` |
| `anonymize` | `Anonymize` / `AnonymizeAsync` |
| `doc_id` | `DocId` |
| `get_map` | `GetMap` |
| `delete` | `Delete` |
| `entities_found` | `EntitiesFound` |
| `llm_review` | `LlmReview` |

Teldeki alan adları Python ile aynıdır: `doc_id`, `placeholder_style`, `keep_legal_form`.

## Kurulum

NuGet adı `bb-mask`, ad alanı `BigBrains`. Paket yayınlanınca:

```bash
dotnet add package bb-mask
```

Bu depoyu çektikten sonra proje dosyasına:

```xml
<ProjectReference Include="yol/bb-mask-sdk/csharp/BbMask.csproj" />
```

Test, `csharp` klasöründen:

```bash
dotnet test BbMask.Tests/BbMask.Tests.csproj
```

Anahtarı koda veya repoya yazmayın.

```bash
export BB_ANONYM_TOKEN="dm_test_..."
```

`Init` süreçte bir kez çalışır. Model çağrısı değildir. Modül metotları bundan önce çağrılırsa `BBError` 401 fırlatır ve servise gitmez.

```csharp
var istemci = BbMask.Init(
    Environment.GetEnvironmentVariable("BB_ANONYM_TOKEN"),
    "https://app.bbmask.co",
    TimeSpan.FromSeconds(150));
```

| Argüman | Anlam |
|---|---|
| `apiKey` | Konsoldaki `dm_test_` anahtarı. Boşsa veya yalnız boşluksa 401. Kurulu istemci değişmez. |
| `baseUrl` | Servis adresi. Boşsa `https://app.bbmask.co`. |
| `timeout` | HTTP zaman aşımı. Boşsa 150 saniye; motor 120 sn bekler, istemci ondan sonra pes eder. |

Dönüş `BBAnonym`'dur. İkinci `Init` öncekinin yerine geçer.

İkinci bir anahtar gerekiyorsa `Init` kullanmadan kurun. ASP.NET Core'da bu nesne singleton olabilir.

```csharp
var diger = new BBAnonym("https://app.bbmask.co", token, TimeSpan.FromSeconds(150));
builder.Services.AddSingleton(diger);
```

## Nesneler

### `Tagged`

`Anonymize`, `AnonymizeFile` ve `AnonymizeDocuments` bunu döner.

| Alan | Ne zaman dolu |
|---|---|
| `Text` | Her zaman. Kişisel veri yerine etiket vardır. |
| `DocId` | `Persist` true (varsayılan). Yoksa `null`. |
| `State` | `Persist` false. Haritanın kendisi. Servise yazılmaz. |
| `EntitiesFound` | Tip başına sayı. Düz isim taşımaz. |
| `BalanceCredits` | Çağrıdan sonra kalan bakiye: `"9.00"`. |
| `RequestId` | Yanıtın `X-Request-Id` değeri. |
| `PlaceholderStyle` | `"angle"` veya `"bracket_tr"`. |
| `KeepLegalForm` | Uygulanan değer. |
| `HealthData` | Uygulanan değer. |
| `LlmReview` | `LlmReview` true istendiyse durum özeti. Düz isim taşımaz. |

### Seçenekler

`AnonymizeOptions`. `Persist` varsayılanı `true`, etiket biçimi `"angle"`.

| Alan | Varsayılan | Etki |
|---|---|---|
| `Persist` | `true` | `false` ise harita servise yazılmaz, `DocId` null kalır. |
| `PlaceholderStyle` | `"angle"` | `"bracket_tr"` → `[KİŞİ_1]`. Başka değer istek gitmeden 400. |
| `KeepLegalForm` | `false` | `true` ise şirket türü eki etiketin dışında kalır. |
| `HealthData` | `false` | `true` ise sağlık verisi de etiketlenir. |
| `LlmReview` | `false` | `true` ise ikinci bir LLM geçişi istenir. |

Sabitler: `BbMask.StyleAngle`, `BbMask.StyleBracketTr`.

```csharp
var tagged = await BbMask.AnonymizeAsync(metin, new AnonymizeOptions
{
    PlaceholderStyle = BbMask.StyleBracketTr,
    KeepLegalForm = true,
    HealthData = true,
    LlmReview = true,
});
```

### `BBError`

`StatusCode`, `Detail`, `RequestId`. Ağ hatası da `BBError` olur.

| Kod | Anlam |
|---|---|
| 400 | Geçersiz `placeholder_style`, 1–3 dışı dosya sayısı, oturumda anonymize yok, `Restore` ne string ne sözlük aldı. |
| 401 | `Init` çağrılmadı, anahtar boş veya tanınmıyor. |
| 402 | Bakiye yok. |
| 404 | Harita yok. |
| 413 | Dosya 20 MB üstü. |
| 502 | Yanıt JSON değil. |
| 503 | Servise ulaşılamadı. |
| 504 | Zaman aşımı doldu. İstek servise ulaşmış olabilir. |

## Çağrılar

Dosya uzantısı `.txt`, `.pdf` veya `.docx`. Dosya başına tavan 20 MB.

`AnonymizeFile(path)` yolu okur. Bayt sizdeyse `AnonymizeBytes(filename, content)` kullanın; iki ad `BbMask` ve `BBAnonym` üzerinde aynı anlama gelir.

### Harita serviste

```csharp
var tagged = await BbMask.AnonymizeAsync(metin);
var duz = await BbMask.RestoreAsync(tagged.DocId, cevap);
await BbMask.DeleteAsync(tagged.DocId);
```

Tek dosya ve birden çok dosya (üst sınır 3, tek `DocId`, tek ücret):

```csharp
var tagged = await BbMask.AnonymizeFileAsync("dilekce.pdf");
var coklu = await BbMask.AnonymizeDocumentsAsync(new[] { "dilekce.pdf", "cevap.docx", "ek.txt" });
```

### Sözlük

```csharp
var sozluk = await BbMask.GetMapAsync(tagged.DocId, once: false);
var silinerek = await BbMask.GetMapAsync(tagged.DocId, once: true);
```

`once: true` okuyunca servisteki kopyayı da siler. Sözlük düz isim taşır; modele yalnız `tagged.Text` gider.

### Harita sizde

```csharp
var tagged = await BbMask.AnonymizeAsync(metin, new AnonymizeOptions { Persist = false });
var duz = await BbMask.RestoreAsync(tagged.State, cevap);
```

### Oturum

`DocId` ve `State` oturumun içindedir. `Dispose` çıkışta çalışır: `Persist` true ise tutulan her kimlik silinir. Silme `BBError` fırlatırsa yutulur.

```csharp
await using var s = BbMask.Session();
var tagged = await s.AnonymizeAsync(metin);
var duz = await s.RestoreAsync(cevap);
```

`BBAnonym` eşzamanlı çağrılabilir. Aynı `MaskSession` sırayla kullanılır.

## Referans

```csharp
Init(apiKey, baseUrl = null, timeout = null) -> BBAnonym
Anonymize / AnonymizeAsync(text, options) -> Tagged
AnonymizeFile / AnonymizeFileAsync(path, options) -> Tagged
AnonymizeBytes / AnonymizeBytesAsync(filename, content, options) -> Tagged
AnonymizeDocuments / AnonymizeDocumentsAsync(paths, options) -> Tagged
Restore / RestoreAsync(docIdOrState, modelText) -> string
GetMap / GetMapAsync(docId, once = false) -> Dictionary<string, object?>
Delete / DeleteAsync(docId) -> bool
Session(options) -> MaskSession
```

Async metotlar sonda `CancellationToken` alır. `BBAnonym` kurucu `Init` istemez; aynı metotları taşır. Anahtar dışa açık özellik değildir; `ToString()` maskeler.

`MaskSession`: `Anonymize`, `AnonymizeFile`, `AnonymizeDocuments`, `Restore`, `Close`. `Dispose` ve `DisposeAsync` `Close` çağırır.

## Bu pakette olmayanlar

- Müşterinin LLM çağrısı.
- Anahtar üretimi ve bakiye. Onlar panelde.
- Etiket onarımı. Bozuk etiketi servis düzeltir.
- ASP.NET controller veya DI eklentisi. İstemciyi siz kaydedersiniz.
