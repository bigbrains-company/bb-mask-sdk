# bb-mask

Türkçe hukuki metindeki kişisel veriyi etiketler, model cevabındaki etiketleri geri açar. Paket ince bir HTTP istemcisidir. Model çağırmaz.

Müşteri Data Mask konsolundan `dm_test_` anahtarı alır. Kendi backend’i bu paketi kurar ve anahtarı `Init`’e verir. LLM satırı müşterinindir; SDK’nın öncesine `Anonymize`, sonrasına `Restore` konur.

Go 1.22+. Bağımlılık yok; `net/http` yeter. Python paketi `python/` klasöründedir. Node, C# ve Java aynı deponun diğer klasörlerindedir.

```go
_, err := bbmask.Init(os.Getenv("BB_ANONYM_TOKEN"))
tagged, err := bbmask.Anonymize("Davacı Hasan Demirkol, TC 62601815964")
cevap, err := kendiLLM(tagged.Text)
duz, err := bbmask.Restore(tagged.DocID, cevap)
_, err = bbmask.Delete(tagged.DocID)
```

Python ve Node hata fırlatır. Go aynı durumu `error` olarak döner. `errors.As` ile `*BBError` alınır. Python’daki `aanonymize` ailesi yoktur; çağrı bloklar, eşzamanlılık `go` ile kurulur.

| Python | Node | Go |
|---|---|---|
| `init` | `init` | `Init` |
| `anonymize` | `anonymize` | `Anonymize` |
| `doc_id` | `docId` | `DocID` |
| `get_map` | `getMap` | `GetMap` |
| `delete` | `deleteMapping` | `Delete` |
| `entities_found` | `entitiesFound` | `EntitiesFound` |
| `llm_review` | `llmReview` | `LLMReview` |

Teldeki alan adları Python ile aynıdır: `doc_id`, `placeholder_style`, `keep_legal_form`.

## Kurulum

Modül yolu `github.com/bigbrains/bb-mask-sdk/go`, paket adı `bbmask`. Yayın deposunun adresi değişirse `go.mod` ilk satırı da değişir.

Paket yayınlanınca:

```bash
go get github.com/bigbrains/bb-mask-sdk/go@v0.2.0
```

Bu depoyu çektikten sonra tüketen modülün `go.mod` dosyasına:

```text
require github.com/bigbrains/bb-mask-sdk/go v0.2.0

replace github.com/bigbrains/bb-mask-sdk/go => /yol/bb-mask-sdk/go
```

`replace` sağındaki yol, bu `go` klasörünün diskteki yeridir. Depo kökünde modül varsa `./bb-mask-sdk/go` yazılır.

Test, bu klasörden:

```bash
go test ./...
```

Anahtarı koda veya repoya yazmayın.

```bash
export BB_ANONYM_TOKEN="dm_test_..."
```

```go
import "github.com/bigbrains/bb-mask-sdk/go"
```

`Init` süreçte bir kez çalışır. Model çağrısı değildir. Modül fonksiyonları bundan önce çağrılırsa `BBError` 401 döner ve servise gitmez.

```go
istemci, err := bbmask.Init(os.Getenv("BB_ANONYM_TOKEN"), bbmask.Config{
    BaseURL: "https://app.bbmask.co", // varsayılan budur
    Timeout: 120 * time.Second,         // varsayılan budur
})
```

| Alan | Anlam |
|---|---|
| `apiKey` | Konsoldaki `dm_test_` anahtarı. Boşsa veya yalnız boşluksa 401. Kurulu istemci değişmez. |
| `BaseURL` | Servis adresi. Boşsa `https://app.bbmask.co`. |
| `Timeout` | HTTP zaman aşımı. Sıfırsa 120 saniye. |

Dönüş `*BBAnonym`’dur. Modül fonksiyonları bu istemciyi kullanır. İkinci `Init` öncekinin yerine geçer.

İkinci bir anahtar gerekiyorsa `Init` kullanmadan `New` kurun.

```go
diger := bbmask.New("https://app.bbmask.co", token, 120*time.Second)
```

## Nesneler

### `Tagged`

`Anonymize`, `AnonymizeFile` ve `AnonymizeDocuments` bunu döner.

| Alan | Ne zaman dolu |
|---|---|
| `Text` | Her zaman. Kişisel veri yerine etiket vardır. |
| `DocID` | `WithPersist(true)` (varsayılan). Boş string, Python’daki `None` karşılığıdır. |
| `State` | `WithPersist(false)`. Haritanın kendisi. Servise yazılmaz. |
| `EntitiesFound` | Tip başına sayı: `map[string]int{"PERSON": 2}`. Düz isim taşımaz. |
| `BalanceCredits` | Çağrıdan sonra kalan bakiye: `"9.00"`. |
| `RequestID` | Yanıtın `X-Request-Id` değeri. |
| `PlaceholderStyle` | `"angle"` veya `"bracket_tr"`. |
| `KeepLegalForm` | Uygulanan değer. |
| `HealthData` | Uygulanan değer. |
| `LLMReview` | `WithLLMReview(true)` istendiyse durum özeti. Düz isim taşımaz. |

### Seçenekler

Hepsi kapalı başlar. `persist` varsayılanı `true`, etiket biçimi `"angle"`.

| Fonksiyon | Varsayılan | Etki |
|---|---|---|
| `WithPersist` | `true` | `false` ise harita servise yazılmaz, `DocID` boş kalır. |
| `WithPlaceholderStyle` | `"angle"` | `"bracket_tr"` → `[KİŞİ_1]`. Başka değer istek gitmeden 400. |
| `WithKeepLegalForm` | `false` | `true` ise şirket türü eki etiketin dışında kalır. |
| `WithHealthData` | `false` | `true` ise sağlık verisi de etiketlenir. |
| `WithLLMReview` | `false` | `true` ise ikinci bir LLM geçişi istenir. |

Sabitler: `StyleAngle`, `StyleBracketTR`.

```go
tagged, err := bbmask.Anonymize(metin,
    bbmask.WithPlaceholderStyle(bbmask.StyleBracketTR),
    bbmask.WithKeepLegalForm(true),
    bbmask.WithHealthData(true),
    bbmask.WithLLMReview(true),
)
```

### `BBError`

`StatusCode`, `Detail`, `RequestID`. `err.Error()` ayrıntı metnidir. Ağ hatası da `*BBError` olur; `net/http` hatası dışarı sızmaz.

```go
var bb *bbmask.BBError
if errors.As(err, &bb) && bb.StatusCode == 402 {
    // bakiye yok
}
```

| Kod | Anlam |
|---|---|
| 400 | Geçersiz `placeholder_style`, 1–3 dışı dosya sayısı, oturumda anonymize yok, `Restore` ne string ne map aldı. |
| 401 | `Init` çağrılmadı, anahtar boş veya tanınmıyor. |
| 402 | Bakiye yok. |
| 404 | Harita yok. |
| 413 | Dosya 20 MB üstü. |
| 501 | `Complete` ailesi. Bu uç henüz açık değil. |
| 502 | Yanıt JSON değil. |
| 503 | Servise ulaşılamadı. |
| 504 | `Timeout` doldu. İstek servise ulaşmış olabilir. |

## Çağrılar

Dosya uzantısı `.txt`, `.pdf` veya `.docx`. Dosya başına tavan 20 MB.

Modül düzeyinde `AnonymizeFile(path)` yolu okur. Bayt sizdeyse `BBAnonym.AnonymizeFile(filename, content)` kullanın. Yolu okuyan metot `AnonymizePath`’tir.

### Harita serviste

```go
tagged, err := bbmask.Anonymize(metin)
duz, err := bbmask.Restore(tagged.DocID, cevap)
_, err = bbmask.Delete(tagged.DocID)
```

Tek dosya ve birden çok dosya (üst sınır 3, tek `DocID`, tek ücret):

```go
tagged, err := bbmask.AnonymizeFile("dilekce.pdf")
coklu, err := bbmask.AnonymizeDocuments([]string{"dilekce.pdf", "cevap.docx", "ek.txt"})
```

### Sözlük

```go
sozluk, err := bbmask.GetMap(tagged.DocID, false)
silinerek, err := bbmask.GetMap(tagged.DocID, true)
```

`true` okuyunca servisteki kopyayı da siler. Sözlük düz isim taşır; modele yalnız `tagged.Text` gider.

### Harita sizde

```go
tagged, err := bbmask.Anonymize(metin, bbmask.WithPersist(false))
duz, err := bbmask.Restore(tagged.State, cevap)
```

### Oturum

`DocID` ve `State` oturumun içindedir. `Close` çıkışta çalışır: `Persist` true ise tutulan her kimlik silinir. Silme `BBError` dönerse yutulur.

```go
s, err := bbmask.Session()
if err != nil {
    return err
}
defer s.Close()

tagged, err := s.Anonymize(metin)
duz, err := s.Restore(cevap)
```

`BBAnonym` eşzamanlı çağrılabilir. Aynı `MaskSession` sırayla kullanılır.

```go
s, err := bbmask.Session(bbmask.WithPersist(false), bbmask.WithPlaceholderStyle("bracket_tr"))
```

### `Complete`

`Complete`, `CompleteFile` ve `CompleteDocuments` HTTP çağırmaz. `Init` yoksa 401, varsa 501. `BBAnonym` üzerindeki aynı metotlar doğrudan 501 döner.

## Referans

```go
Init(apiKey string, cfg ...Config) (*BBAnonym, error)
Anonymize(text string, opts ...Option) (*Tagged, error)
AnonymizeFile(path string, opts ...Option) (*Tagged, error)
AnonymizeDocuments(paths []string, opts ...Option) (*Tagged, error)
Restore(docIDOrState any, modelText string) (string, error)
GetMap(docID string, once bool) (map[string]any, error)
Delete(docID string) (bool, error)
Session(opts ...Option) (*MaskSession, error)
Complete(text string) (string, error)
CompleteFile(path string) (string, error)
CompleteDocuments(paths []string) (string, error)
```

`New` kurucu `Init` istemez. Dosya burada ikiye ayrılır: `AnonymizeFile(filename, content)` bayt alır, `AnonymizePath(path)` yolu okur.

`MaskSession`: `Anonymize`, `AnonymizeFile`, `AnonymizeDocuments`, `Restore`, `Close`.

## Bu pakette olmayanlar

- Müşterinin LLM çağrısı.
- Anahtar üretimi ve bakiye. Onlar panelde.
- Etiket onarımı. Bozuk etiketi servis düzeltir.
- Python’daki `aanonymize` ailesi. Go çağrısı bloklar.
