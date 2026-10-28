# bb-mask

Türkçe hukuki metindeki kişisel veriyi etiketler, model cevabındaki etiketleri geri açar. Paket ince bir HTTP istemcisidir. Model çağırmaz.

Müşteri Data Mask konsolundan `dm_test_` anahtarı alır. Kendi backend’i bu paketi kurar ve anahtarı `init`’e verir. LLM satırı müşterinindir; SDK’nın öncesine `anonymize`, sonrasına `restore` konur.

Node 18+. Bağımlılık yok; `fetch` yeter. Paket ESM’dir; `import` kullanılır.

```js
import { init, anonymize, restore, deleteMapping } from "bb-mask";

init(process.env.BB_ANONYM_TOKEN);

const tagged = await anonymize("Davacı Hasan Demirkol, TC 62601815964");
const cevap = await kendiLlm(tagged.text);
const duz = await restore(tagged.docId, cevap);
await deleteMapping(tagged.docId);
```

## Kurulum

```bash
npm install bb-mask
```

Anahtarı koda veya depoya yazmayın.

```bash
export BB_ANONYM_TOKEN="dm_test_..."
```

`init` süreçte bir kez çalışır. Model çağrısı değildir. Modül fonksiyonları bundan önce çağrılırsa `BBError(401)` verir ve servise gitmez.

```js
const istemci = init(process.env.BB_ANONYM_TOKEN, {
  baseUrl: "https://app.bbmask.co",
  timeout: 150,
});
```

| Argüman | Anlam |
|---|---|
| `apiKey` | Konsoldaki `dm_test_` anahtarı. Boşsa `BBError(401)`. |
| `baseUrl` | Servis adresi. Varsayılan `https://app.bbmask.co`. |
| `timeout` | HTTP zaman aşımı, saniye. Varsayılan `150`; motor 120 sn bekler, istemci ondan sonra pes eder. |

İkinci bir anahtar gerekiyorsa `init` kullanmadan `BBAnonym` kurun.

```js
import { BBAnonym } from "bb-mask";

const diger = new BBAnonym("https://app.bbmask.co", token, 150);
```

## Nesneler

### `Tagged`

`anonymize`, `anonymizeFile` ve `anonymizeDocuments` bunu döner.

| Alan | Ne zaman dolu |
|---|---|
| `text` | Her zaman. Kişisel veri yerine etiket vardır. |
| `docId` | `persist: true` (varsayılan). Haritanın servisteki anahtarı. |
| `state` | `persist: false`. Haritanın kendisi. Servise yazılmaz. |
| `entitiesFound` | Tip başına sayı: `{ PERSON: 2 }`. Düz isim taşımaz. |
| `balanceCredits` | Çağrıdan sonra kalan bakiye: `"9.00"`. |
| `requestId` | Yanıtın `X-Request-Id` değeri. |
| `placeholderStyle` | `"angle"` veya `"bracket_tr"`. |
| `keepLegalForm` | Uygulanan değer. |
| `healthData` | Uygulanan değer. |
| `llmReview` | `llmReview: true` istendiyse durum özeti. Düz isim taşımaz. |

### Seçenekler

İkinci argüman bir nesnedir. Hepsi kapalı başlar.

| Alan | Varsayılan | Etki |
|---|---|---|
| `persist` | `true` | `false` ise harita servise yazılmaz, `docId` boş kalır. |
| `placeholderStyle` | `"angle"` | `"bracket_tr"` → `[KİŞİ_1]`. Başka değer istek gitmeden `BBError(400)`. |
| `keepLegalForm` | `false` | `true` ise şirket türü eki etiketin dışında kalır. |
| `healthData` | `false` | `true` ise sağlık verisi de etiketlenir. |
| `llmReview` | `false` | `true` ise ikinci bir LLM geçişi istenir. |

Python adları da okunur: `placeholder_style`, `keep_legal_form`, `health_data`, `llm_review`.

### `BBError`

`statusCode`, `detail`, `requestId`. `String(hata)` ayrıntı metnidir. Ağ hatası da `BBError` olur.

| Kod | Anlam |
|---|---|
| 400 | Geçersiz `placeholderStyle`, 1–3 dışı dosya sayısı, oturumda anonymize yok. |
| 401 | `init` çağrılmadı, anahtar boş veya tanınmıyor. |
| 402 | Bakiye yok. |
| 404 | Harita yok. |
| 413 | Dosya 20 MB üstü. |
| 502 | Yanıt JSON değil. |
| 503 | Servise ulaşılamadı. |
| 504 | `timeout` doldu. İstek servise ulaşmış olabilir. |

## Çağrılar

Dosya uzantısı `.txt`, `.pdf` veya `.docx`. Dosya başına tavan 20 MB.

`anonymizeFile(path)` yolu okur. Bayt sizdeyse `anonymizeBytes(filename, content)` kullanın; iki ad modülde ve `BBAnonym` üzerinde aynı anlama gelir. `delete` JavaScript’te ayrılmış sözcük olduğu için silme metodu `deleteMapping`’dir.

### Harita serviste

```js
const tagged = await anonymize(metin);
const duz = await restore(tagged.docId, await kendiLlm(tagged.text));
await deleteMapping(tagged.docId);
```

Tek dosya ve birden çok dosya (üst sınır 3, tek `docId`, tek ücret):

```js
const tagged = await anonymizeFile("dilekce.pdf");
const bayt = await anonymizeBytes("dilekce.pdf", buffer);
const coklu = await anonymizeDocuments(["dilekce.pdf", "cevap.docx", "ek.txt"]);
```

### Sözlük

```js
const sozluk = await getMap(tagged.docId);
const silinerek = await getMap(tagged.docId, true);
```

`true` okuyunca servisteki kopyayı da siler. Sözlük düz isim taşır; modele yalnız `tagged.text` gider.

### Harita sizde

```js
const tagged = await anonymize(metin, { persist: false });
const duz = await restore(tagged.state, await kendiLlm(tagged.text));
```

### Oturum

`docId` ve `state` oturumun içindedir. `close` çıkışta çalışır: `persist: true` ise tutulan her kimlik silinir.

```js
const s = session();
try {
  const tagged = await s.anonymize(metin);
  const duz = await s.restore(await kendiLlm(tagged.text));
} finally {
  await s.close();
}
```

Node `await using` destekliyorsa `await using s = session()` aynı `close` çağrısını yapar.

```js
const s = session({ persist: false, placeholderStyle: "bracket_tr" });
```

## Referans

```js
init(apiKey, { baseUrl, timeout } = {})
anonymize(text, options) -> Promise<Tagged>
anonymizeFile(path, options) -> Promise<Tagged>
anonymizeBytes(filename, content, options) -> Promise<Tagged>
anonymizeDocuments(paths, options) -> Promise<Tagged>
restore(docIdOrState, modelText) -> Promise<string>
getMap(docId, once = false) -> Promise<object>
deleteMapping(docId) -> Promise<boolean>
session(options) -> MaskSession
```

`BBAnonym` kurucu `init` istemez; aynı metotları taşır. `token` alanı sayılmaz (non-enumerable): `console.log` ve `JSON.stringify` anahtarı göstermez. Paket `index.d.ts` ile tip tanımı taşır.
