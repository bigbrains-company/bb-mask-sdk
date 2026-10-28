# bb-mask

Türkçe hukuki metindeki kişisel veriyi etiketler, model cevabındaki etiketleri geri açar. Paket ince bir HTTP istemcisidir. Model çağırmaz.

Müşteri Data Mask konsolundan `dm_test_` anahtarı alır. Java veya Spring servisi bu paketi kurar ve anahtarı `init`'e verir. LLM satırı müşterinindir; SDK'nın öncesine `anonymize`, sonrasına `restore` konur. Paket Spring'e bağlanmaz.

Java 17+. Çalışma bağımlılığı `jackson-databind`. Python, Node, Go ve C# aynı deponun diğer klasörlerindedir.

```java
import com.bigbrains.bbmask.BbMask;
import com.bigbrains.bbmask.Tagged;

BbMask.init(System.getenv("BB_ANONYM_TOKEN"));

Tagged tagged = BbMask.anonymize("Davacı Hasan Demirkol, TC 62601815964");
String cevap = kendiLlm(tagged.text());
String duz = BbMask.restore(tagged.docId(), cevap);
BbMask.delete(tagged.docId());
```

Hatalar `BBError` fırlatır. `getMessage()` ayrıntı metnidir. Çağrı bloklar; Spring WebFlux kullanıyorsanız kendi zamanlayıcınıza alın.

| Python | Java |
|---|---|
| `init` | `BbMask.init` |
| `anonymize` | `anonymize` |
| `doc_id` | `docId()` |
| `get_map` | `getMap` |
| `delete` | `delete` |
| `entities_found` | `entitiesFound()` |
| `llm_review` | `llmReview()` |

Teldeki alan adları Python ile aynıdır: `doc_id`, `placeholder_style`, `keep_legal_form`.

## Kurulum

Maven koordinatı `tr.com.bigbrains:bb-mask:0.3.0`. Grup kimliği Central Portal’da doğrulanmış `tr.com.bigbrains` ad alanıdır; Java paket adı `com.bigbrains.bbmask` kalır.

Paket yayınlanınca:

```xml
<dependency>
  <groupId>tr.com.bigbrains</groupId>
  <artifactId>bb-mask</artifactId>
  <version>0.3.0</version>
</dependency>
```

Bu depoyu çektikten sonra test, `java` klasöründen:

```bash
mvn test
```

Anahtarı koda veya repoya yazmayın.

```bash
export BB_ANONYM_TOKEN="dm_test_..."
```

`init` süreçte bir kez çalışır. Model çağrısı değildir. Sınıf metotları bundan önce çağrılırsa `BBError` 401 fırlatır ve servise gitmez.

```java
BBAnonym istemci = BbMask.init(
    System.getenv("BB_ANONYM_TOKEN"),
    "https://app.bbmask.co",
    Duration.ofSeconds(150));
```

| Argüman | Anlam |
|---|---|
| `apiKey` | Konsoldaki `dm_test_` anahtarı. Boşsa veya yalnız boşluksa 401. Kurulu istemci değişmez. |
| `baseUrl` | Servis adresi. Boşsa `https://app.bbmask.co`. |
| `timeout` | HTTP zaman aşımı. Boşsa 150 saniye; motor 120 sn bekler, istemci ondan sonra pes eder. |

Dönüş `BBAnonym`'dur. İkinci `init` öncekinin yerine geçer.

İkinci bir anahtar gerekiyorsa `init` kullanmadan kurun. Spring'de bu nesne bir `@Bean` olabilir.

```java
@Bean
BBAnonym bbAnonym(@Value("${bb.anonym.token}") String token) {
    return new BBAnonym("https://app.bbmask.co", token, Duration.ofSeconds(150));
}
```

## Nesneler

### `Tagged`

`anonymize`, `anonymizeFile` ve `anonymizeDocuments` bunu döner. Erişim `text()`, `docId()` biçimindedir.

| Alan | Ne zaman dolu |
|---|---|
| `text` | Her zaman. Kişisel veri yerine etiket vardır. |
| `docId` | `persist` true (varsayılan). Yoksa `null`. |
| `state` | `persist` false. Haritanın kendisi. Servise yazılmaz. |
| `entitiesFound` | Tip başına sayı. Düz isim taşımaz. |
| `balanceCredits` | Çağrıdan sonra kalan bakiye: `"9.00"`. |
| `requestId` | Yanıtın `X-Request-Id` değeri. |
| `placeholderStyle` | `"angle"` veya `"bracket_tr"`. |
| `keepLegalForm` | Uygulanan değer. |
| `healthData` | Uygulanan değer. |
| `llmReview` | `llmReview` true istendiyse durum özeti. Düz isim taşımaz. |

### Seçenekler

`Options`. `persist` varsayılanı `true`, etiket biçimi `"angle"`.

| Alan | Varsayılan | Etki |
|---|---|---|
| `persist` | `true` | `false` ise harita servise yazılmaz, `docId` null kalır. |
| `placeholderStyle` | `"angle"` | `"bracket_tr"` → `[KİŞİ_1]`. Başka değer istek gitmeden 400. |
| `keepLegalForm` | `false` | `true` ise şirket türü eki etiketin dışında kalır. |
| `healthData` | `false` | `true` ise sağlık verisi de etiketlenir. |
| `llmReview` | `false` | `true` ise ikinci bir LLM geçişi istenir. |

Sabitler: `BbMask.STYLE_ANGLE`, `BbMask.STYLE_BRACKET_TR`.

```java
Tagged tagged = BbMask.anonymize(metin, new Options()
    .placeholderStyle(BbMask.STYLE_BRACKET_TR)
    .keepLegalForm(true)
    .healthData(true)
    .llmReview(true));
```

### `BBError`

`getStatusCode()`, `getDetail()`, `getRequestId()`. Ağ hatası da `BBError` olur.

| Kod | Anlam |
|---|---|
| 400 | Geçersiz `placeholder_style`, 1–3 dışı dosya sayısı, oturumda anonymize yok, `restore` ne string ne map aldı. |
| 401 | `init` çağrılmadı, anahtar boş veya tanınmıyor. |
| 402 | Bakiye yok. |
| 404 | Harita yok. |
| 413 | Dosya 20 MB üstü. |
| 502 | Yanıt JSON değil. |
| 503 | Servise ulaşılamadı. |
| 504 | Zaman aşımı doldu. İstek servise ulaşmış olabilir. |

## Çağrılar

Dosya uzantısı `.txt`, `.pdf` veya `.docx`. Dosya başına tavan 20 MB.

`anonymizeFile(path)` yolu okur. Bayt sizdeyse `anonymizeBytes(filename, content)` kullanın; iki ad `BbMask` ve `BBAnonym` üzerinde aynı anlama gelir.

### Harita serviste

```java
Tagged tagged = BbMask.anonymize(metin);
String duz = BbMask.restore(tagged.docId(), cevap);
BbMask.delete(tagged.docId());
```

Tek dosya ve birden çok dosya (üst sınır 3, tek `docId`, tek ücret):

```java
Tagged tagged = BbMask.anonymizeFile("dilekce.pdf");
Tagged coklu = BbMask.anonymizeDocuments(List.of("dilekce.pdf", "cevap.docx", "ek.txt"));
```

### Sözlük

```java
Map<String, Object> sozluk = BbMask.getMap(tagged.docId(), false);
Map<String, Object> silinerek = BbMask.getMap(tagged.docId(), true);
```

`true` okuyunca servisteki kopyayı da siler. Sözlük düz isim taşır; modele yalnız `tagged.text()` gider.

### Harita sizde

```java
Tagged tagged = BbMask.anonymize(metin, new Options().persist(false));
String duz = BbMask.restore(tagged.state(), cevap);
```

### Oturum

`docId` ve `state` oturumun içindedir. `close` çıkışta çalışır: `persist` true ise tutulan her kimlik silinir. Silme `BBError` fırlatırsa yutulur. `try-with-resources` aynı `close` çağrısını yapar.

```java
try (MaskSession s = BbMask.session()) {
    Tagged tagged = s.anonymize(metin);
    String duz = s.restore(cevap);
}
```

```java
try (MaskSession s = BbMask.session(new Options().persist(false).placeholderStyle("bracket_tr"))) {
    Tagged tagged = s.anonymize(metin);
}
```

## Referans

```java
BbMask.init(apiKey)
BbMask.init(apiKey, baseUrl, timeout)
anonymize(text, options) -> Tagged
anonymizeFile(path, options) -> Tagged
anonymizeBytes(filename, content, options) -> Tagged
anonymizeDocuments(paths, options) -> Tagged
restore(docIdOrState, modelText) -> String
getMap(docId, once) -> Map<String, Object>
delete(docId) -> boolean
session(options) -> MaskSession
```

`BBAnonym` kurucu `init` istemez; aynı metotları taşır. Anahtar dışa açık değildir; `toString()` maskeler.

`MaskSession`: `anonymize`, `anonymizeFile`, `anonymizeDocuments`, `restore`, `close`.

## Bu pakette olmayanlar

- Müşterinin LLM çağrısı.
- Anahtar üretimi ve bakiye. Onlar panelde.
- Etiket onarımı. Bozuk etiketi servis düzeltir.
- Spring Boot starter. İstemciyi kendi `@Bean`'inizde kurarsınız.
