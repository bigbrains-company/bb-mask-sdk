using System.Globalization;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;

namespace BigBrains;

/// <summary>
/// Data Mask anonimleştirme servisinin .NET istemcisi. Model çağırmaz.
/// </summary>
public static class BbMask
{
    public const string Version = "0.2.0";
    public const string DefaultBaseUrl = "http://43.229.92.93:8081";
    public static readonly TimeSpan DefaultTimeout = TimeSpan.FromSeconds(120);
    public const string StyleAngle = "angle";
    public const string StyleBracketTr = "bracket_tr";

    private static readonly object Gate = new();
    private static BBAnonym? _current;

    /// <summary>Süreç istemcisini kurar. İkinci çağrı öncekinin yerine geçer. Boş anahtar 401 fırlatır.</summary>
    public static BBAnonym Init(string apiKey, string? baseUrl = null, TimeSpan? timeout = null)
    {
        var key = (apiKey ?? "").Trim();
        if (key.Length == 0)
            throw new BBError(401, "api_key boş");
        var client = new BBAnonym(baseUrl, key, timeout);
        lock (Gate)
            _current = client;
        return client;
    }

    internal static void Reset()
    {
        lock (Gate)
            _current = null;
    }

    private static BBAnonym Require()
    {
        lock (Gate)
        {
            if (_current is null)
                throw new BBError(401, "Init çağrılmadı");
            return _current;
        }
    }

    public static Task<Tagged> AnonymizeAsync(string text, AnonymizeOptions? options = null, CancellationToken cancellationToken = default)
        => Require().AnonymizeAsync(text, options, cancellationToken);

    public static Tagged Anonymize(string text, AnonymizeOptions? options = null)
        => AnonymizeAsync(text, options).GetAwaiter().GetResult();

    public static Task<Tagged> AnonymizeFileAsync(string path, AnonymizeOptions? options = null, CancellationToken cancellationToken = default)
        => Require().AnonymizePathAsync(path, options, cancellationToken);

    public static Tagged AnonymizeFile(string path, AnonymizeOptions? options = null)
        => AnonymizeFileAsync(path, options).GetAwaiter().GetResult();

    public static Task<Tagged> AnonymizeDocumentsAsync(IReadOnlyList<string> paths, AnonymizeOptions? options = null, CancellationToken cancellationToken = default)
        => Require().AnonymizeDocumentsAsync(paths, options, cancellationToken);

    public static Tagged AnonymizeDocuments(IReadOnlyList<string> paths, AnonymizeOptions? options = null)
        => AnonymizeDocumentsAsync(paths, options).GetAwaiter().GetResult();

    public static Task<string> RestoreAsync(object? docIdOrState, string modelText, CancellationToken cancellationToken = default)
        => Require().RestoreAsync(docIdOrState, modelText, cancellationToken);

    public static string Restore(object? docIdOrState, string modelText)
        => RestoreAsync(docIdOrState, modelText).GetAwaiter().GetResult();

    public static Task<Dictionary<string, object?>> GetMapAsync(string docId, bool once = false, CancellationToken cancellationToken = default)
        => Require().GetMapAsync(docId, once, cancellationToken);

    public static Dictionary<string, object?> GetMap(string docId, bool once = false)
        => GetMapAsync(docId, once).GetAwaiter().GetResult();

    public static Task<bool> DeleteAsync(string docId, CancellationToken cancellationToken = default)
        => Require().DeleteAsync(docId, cancellationToken);

    public static bool Delete(string docId)
        => DeleteAsync(docId).GetAwaiter().GetResult();

    public static MaskSession Session(AnonymizeOptions? options = null)
        => new(Require(), AnonymizeOptions.Resolve(options));

    public static Task<string> CompleteAsync(string text, CancellationToken cancellationToken = default)
        => Require().CompleteAsync(text, cancellationToken);

    public static string Complete(string text) => Require().Complete(text);

    public static Task<string> CompleteFileAsync(string path, CancellationToken cancellationToken = default)
        => Require().CompleteFileAsync(path, cancellationToken);

    public static string CompleteFile(string path) => Require().CompleteFile(path);

    public static Task<string> CompleteDocumentsAsync(IReadOnlyList<string> paths, CancellationToken cancellationToken = default)
        => Require().CompleteDocumentsAsync(paths, cancellationToken);

    public static string CompleteDocuments(IReadOnlyList<string> paths) => Require().CompleteDocuments(paths);
}

/// <summary>Servis veya istemci hatası. <see cref="Exception.Message"/> ayrıntı metnidir.</summary>
public sealed class BBError : Exception
{
    public int StatusCode { get; }
    public string Detail { get; }
    public string? RequestId { get; }

    public BBError(int statusCode, string detail, string? requestId = null) : base(detail)
    {
        StatusCode = statusCode;
        Detail = detail;
        RequestId = requestId;
    }
}

/// <summary>anonymize çağrısının dönüşü.</summary>
public sealed class Tagged
{
    public string Text { get; init; } = "";
    public string? DocId { get; init; }
    public Dictionary<string, object?>? State { get; init; }
    public Dictionary<string, int> EntitiesFound { get; init; } = new();
    public string? BalanceCredits { get; init; }
    public string? RequestId { get; init; }
    public string PlaceholderStyle { get; init; } = BbMask.StyleAngle;
    public bool KeepLegalForm { get; init; }
    public bool HealthData { get; init; }
    public Dictionary<string, object?>? LlmReview { get; init; }
}

/// <summary>Anonymize ve Session seçenekleri. persist varsayılanı true, etiket biçimi angle.</summary>
public sealed class AnonymizeOptions
{
    public bool Persist { get; set; } = true;
    public string PlaceholderStyle { get; set; } = BbMask.StyleAngle;
    public bool KeepLegalForm { get; set; }
    public bool HealthData { get; set; }
    public bool LlmReview { get; set; }

    internal AnonymizeOptions Copy() => new()
    {
        Persist = Persist,
        PlaceholderStyle = string.IsNullOrWhiteSpace(PlaceholderStyle) ? BbMask.StyleAngle : PlaceholderStyle,
        KeepLegalForm = KeepLegalForm,
        HealthData = HealthData,
        LlmReview = LlmReview,
    };

    internal static AnonymizeOptions Resolve(AnonymizeOptions? options)
    {
        var copy = options?.Copy() ?? new AnonymizeOptions();
        if (copy.PlaceholderStyle != BbMask.StyleAngle && copy.PlaceholderStyle != BbMask.StyleBracketTr)
            throw new BBError(400, "placeholder_style angle veya bracket_tr olmalı");
        return copy;
    }
}

/// <summary>Tek anahtarın istemcisi. Init gerekmez.</summary>
public sealed class BBAnonym : IDisposable
{
    private const string NotOpen = "Bu uç henüz açık değil.";

    public string BaseUrl { get; }
    public string Token { get; }
    public TimeSpan Timeout { get; }
    private readonly HttpClient _http;

    public BBAnonym(string? baseUrl, string? token, TimeSpan? timeout = null)
    {
        var trimmed = (baseUrl ?? "").Trim().TrimEnd('/');
        BaseUrl = trimmed.Length == 0 ? BbMask.DefaultBaseUrl : trimmed;
        Token = token ?? "";
        Timeout = timeout is { } value && value > TimeSpan.Zero ? value : BbMask.DefaultTimeout;
        _http = new HttpClient(new SocketsHttpHandler { UseCookies = false })
        {
            Timeout = Timeout,
        };
        _http.DefaultRequestHeaders.ExpectContinue = false;
    }

    public void Dispose() => _http.Dispose();

    public Task<Tagged> AnonymizeAsync(string text, AnonymizeOptions? options = null, CancellationToken cancellationToken = default)
    {
        var resolved = AnonymizeOptions.Resolve(options);
        var payload = new Dictionary<string, object?>
        {
            ["text"] = text,
            ["persist"] = resolved.Persist,
            ["placeholder_style"] = resolved.PlaceholderStyle,
            ["keep_legal_form"] = resolved.KeepLegalForm,
            ["health_data"] = resolved.HealthData,
            ["llm_review"] = resolved.LlmReview,
        };
        return SendTaggedAsync(HttpMethod.Post, "/v1/anonymize", JsonContent(payload), null, resolved.Persist, cancellationToken);
    }

    public Tagged Anonymize(string text, AnonymizeOptions? options = null)
        => AnonymizeAsync(text, options).GetAwaiter().GetResult();

    public async Task<Tagged> AnonymizeFileAsync(string filename, byte[] content, AnonymizeOptions? options = null, CancellationToken cancellationToken = default)
    {
        var resolved = AnonymizeOptions.Resolve(options);
        using var form = Form(resolved, new[] { ("file", filename, content) });
        return await SendTaggedAsync(HttpMethod.Post, "/v1/anonymize/file", form, null, resolved.Persist, cancellationToken).ConfigureAwait(false);
    }

    public Tagged AnonymizeFile(string filename, byte[] content, AnonymizeOptions? options = null)
        => AnonymizeFileAsync(filename, content, options).GetAwaiter().GetResult();

    public async Task<Tagged> AnonymizePathAsync(string path, AnonymizeOptions? options = null, CancellationToken cancellationToken = default)
    {
        var content = await File.ReadAllBytesAsync(path, cancellationToken).ConfigureAwait(false);
        return await AnonymizeFileAsync(Path.GetFileName(path), content, options, cancellationToken).ConfigureAwait(false);
    }

    public Tagged AnonymizePath(string path, AnonymizeOptions? options = null)
        => AnonymizePathAsync(path, options).GetAwaiter().GetResult();

    public async Task<Tagged> AnonymizeDocumentsAsync(IReadOnlyList<string> paths, AnonymizeOptions? options = null, CancellationToken cancellationToken = default)
    {
        var resolved = AnonymizeOptions.Resolve(options);
        if (paths is null || paths.Count is < 1 or > 3)
            throw new BBError(400, "Bir istekte 1–3 dosya");
        var files = new List<(string Field, string Name, byte[] Content)>();
        foreach (var path in paths)
        {
            var content = await File.ReadAllBytesAsync(path, cancellationToken).ConfigureAwait(false);
            files.Add(("files", Path.GetFileName(path), content));
        }
        using var form = Form(resolved, files);
        return await SendTaggedAsync(HttpMethod.Post, "/v1/anonymize/documents", form, null, resolved.Persist, cancellationToken).ConfigureAwait(false);
    }

    public Tagged AnonymizeDocuments(IReadOnlyList<string> paths, AnonymizeOptions? options = null)
        => AnonymizeDocumentsAsync(paths, options).GetAwaiter().GetResult();

    public async Task<string> RestoreAsync(object? docIdOrState, string modelText, CancellationToken cancellationToken = default)
    {
        var payload = new Dictionary<string, object?> { ["text"] = modelText };
        if (docIdOrState is System.Collections.IDictionary map)
            payload["state"] = map;
        else if (docIdOrState is string id)
            payload["doc_id"] = id;
        else
            throw new BBError(400, "restore doc_id (string) veya state (map) ister");
        var body = await SendAsync(HttpMethod.Post, "/v1/restore", JsonContent(payload), null, cancellationToken).ConfigureAwait(false);
        return StringField(body, "text");
    }

    public string Restore(object? docIdOrState, string modelText)
        => RestoreAsync(docIdOrState, modelText).GetAwaiter().GetResult();

    public async Task<Dictionary<string, object?>> GetMapAsync(string docId, bool once = false, CancellationToken cancellationToken = default)
    {
        var body = await SendAsync(HttpMethod.Get, "/v1/mappings/" + Uri.EscapeDataString(docId), null, "once=" + (once ? "true" : "false"), cancellationToken).ConfigureAwait(false);
        if (body.TryGetValue("state", out var value) && value is Dictionary<string, object?> state)
            return state;
        return new Dictionary<string, object?>();
    }

    public Dictionary<string, object?> GetMap(string docId, bool once = false)
        => GetMapAsync(docId, once).GetAwaiter().GetResult();

    public async Task<bool> DeleteAsync(string docId, CancellationToken cancellationToken = default)
    {
        var body = await SendAsync(HttpMethod.Delete, "/v1/mappings/" + Uri.EscapeDataString(docId), null, null, cancellationToken).ConfigureAwait(false);
        return body.TryGetValue("deleted", out var value) && value is true;
    }

    public bool Delete(string docId) => DeleteAsync(docId).GetAwaiter().GetResult();

    public Task<string> CompleteAsync(string text, CancellationToken cancellationToken = default)
        => throw new BBError(501, NotOpen);

    public string Complete(string text) => throw new BBError(501, NotOpen);

    public Task<string> CompleteFileAsync(string path, CancellationToken cancellationToken = default)
        => throw new BBError(501, NotOpen);

    public string CompleteFile(string path) => throw new BBError(501, NotOpen);

    public Task<string> CompleteDocumentsAsync(IReadOnlyList<string> paths, CancellationToken cancellationToken = default)
        => throw new BBError(501, NotOpen);

    public string CompleteDocuments(IReadOnlyList<string> paths) => throw new BBError(501, NotOpen);

    private async Task<Tagged> SendTaggedAsync(HttpMethod method, string path, HttpContent content, string? query, bool persist, CancellationToken cancellationToken)
    {
        var body = await SendAsync(method, path, content, query, cancellationToken).ConfigureAwait(false);
        return ToTagged(body, persist);
    }

    private async Task<Dictionary<string, object?>> SendAsync(HttpMethod method, string path, HttpContent? content, string? query, CancellationToken cancellationToken)
    {
        var url = BaseUrl + path;
        if (!string.IsNullOrEmpty(query))
            url += "?" + query;
        using var request = new HttpRequestMessage(method, url) { Content = content };
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", Token);
        try
        {
            using var response = await _http.SendAsync(request, HttpCompletionOption.ResponseContentRead, cancellationToken).ConfigureAwait(false);
            var bytes = await response.Content.ReadAsByteArrayAsync(cancellationToken).ConfigureAwait(false);
            string? requestId = null;
            if (response.Headers.TryGetValues("X-Request-Id", out var values))
                requestId = values.FirstOrDefault();
            return Parse((int)response.StatusCode, requestId, Encoding.UTF8.GetString(bytes));
        }
        catch (BBError)
        {
            throw;
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
            throw;
        }
        catch (OperationCanceledException)
        {
            throw new BBError(504, TimeoutDetail());
        }
        catch (HttpRequestException)
        {
            throw new BBError(503, Unreachable());
        }
        catch (IOException)
        {
            throw new BBError(503, Unreachable());
        }
    }

    private string TimeoutDetail()
    {
        var seconds = Timeout.TotalSeconds.ToString("0.###", CultureInfo.InvariantCulture);
        return $"Zaman aşımı ({seconds} sn). Ücret düşmüş olabilir.";
    }

    private string Unreachable() => $"Servise ulaşılamadı ({BaseUrl})";

    private static StringContent JsonContent(Dictionary<string, object?> payload)
        => new(JsonSerializer.Serialize(payload), Encoding.UTF8, "application/json");

    private static MultipartFormDataContent Form(AnonymizeOptions options, IEnumerable<(string Field, string Name, byte[] Content)> files)
    {
        var form = new MultipartFormDataContent();
        form.Add(new StringContent(options.Persist ? "true" : "false"), "persist");
        form.Add(new StringContent(options.PlaceholderStyle), "placeholder_style");
        form.Add(new StringContent(options.KeepLegalForm ? "true" : "false"), "keep_legal_form");
        form.Add(new StringContent(options.HealthData ? "true" : "false"), "health_data");
        form.Add(new StringContent(options.LlmReview ? "true" : "false"), "llm_review");
        foreach (var file in files)
        {
            var bytes = new ByteArrayContent(file.Content);
            bytes.Headers.ContentType = new MediaTypeHeaderValue("application/octet-stream");
            form.Add(bytes, file.Field, file.Name);
        }
        return form;
    }

    private static Dictionary<string, object?> Parse(int status, string? requestId, string raw)
    {
        if (status >= 400)
            throw new BBError(status, ErrorDetail(raw), requestId);
        try
        {
            using var doc = JsonDocument.Parse(raw);
            if (doc.RootElement.ValueKind != JsonValueKind.Object)
                throw new BBError(502, "Servis JSON dönmedi", requestId);
            var map = ToMap(doc.RootElement);
            map["_request_id"] = requestId;
            return map;
        }
        catch (BBError)
        {
            throw;
        }
        catch (JsonException)
        {
            throw new BBError(502, "Servis JSON dönmedi", requestId);
        }
    }

    private static string ErrorDetail(string raw)
    {
        try
        {
            using var doc = JsonDocument.Parse(raw);
            if (doc.RootElement.ValueKind != JsonValueKind.Object)
                return raw;
            if (!doc.RootElement.TryGetProperty("detail", out var detail) || detail.ValueKind == JsonValueKind.Null)
                return raw;
            if (detail.ValueKind == JsonValueKind.String)
                return detail.GetString() ?? raw;
            return detail.GetRawText();
        }
        catch (JsonException)
        {
            return raw;
        }
    }

    private static Tagged ToTagged(Dictionary<string, object?> body, bool persist)
    {
        var docId = persist ? NullIfEmpty(StringField(body, "doc_id")) : null;
        var style = StringField(body, "placeholder_style");
        if (style.Length == 0)
            style = BbMask.StyleAngle;
        return new Tagged
        {
            Text = StringField(body, "text"),
            DocId = docId,
            State = body.TryGetValue("state", out var state) ? state as Dictionary<string, object?> : null,
            EntitiesFound = IntMap(body.GetValueOrDefault("entities_found")),
            BalanceCredits = NullIfEmpty(StringField(body, "balance_credits")),
            RequestId = NullIfEmpty(StringField(body, "_request_id")),
            PlaceholderStyle = style,
            KeepLegalForm = body.TryGetValue("keep_legal_form", out var keep) && keep is true,
            HealthData = body.TryGetValue("health_data", out var health) && health is true,
            LlmReview = body.TryGetValue("llm_review", out var review) ? review as Dictionary<string, object?> : null,
        };
    }

    private static Dictionary<string, object?> ToMap(JsonElement element)
    {
        var map = new Dictionary<string, object?>();
        foreach (var property in element.EnumerateObject())
            map[property.Name] = ToValue(property.Value);
        return map;
    }

    private static object? ToValue(JsonElement element) => element.ValueKind switch
    {
        JsonValueKind.Object => ToMap(element),
        JsonValueKind.Array => element.EnumerateArray().Select(ToValue).ToList(),
        JsonValueKind.String => element.GetString(),
        JsonValueKind.Number => element.TryGetInt32(out var integer) ? integer : element.TryGetInt64(out var longer) ? longer : element.GetDouble(),
        JsonValueKind.True => true,
        JsonValueKind.False => false,
        _ => null,
    };

    private static Dictionary<string, int> IntMap(object? value)
    {
        var result = new Dictionary<string, int>();
        if (value is not Dictionary<string, object?> map)
            return result;
        foreach (var pair in map)
        {
            switch (pair.Value)
            {
                case int integer:
                    result[pair.Key] = integer;
                    break;
                case long longer:
                    result[pair.Key] = (int)longer;
                    break;
                case double number:
                    result[pair.Key] = (int)number;
                    break;
            }
        }
        return result;
    }

    private static string StringField(Dictionary<string, object?> body, string key)
        => body.TryGetValue(key, out var value) && value is string text ? text : "";

    private static string? NullIfEmpty(string value) => value.Length == 0 ? null : value;
}

/// <summary>DocId veya state oturumun içindedir. Dispose çıkışta haritayı siler.</summary>
public sealed class MaskSession : IDisposable, IAsyncDisposable
{
    private readonly BBAnonym _client;
    private readonly AnonymizeOptions _options;
    private readonly List<string> _ids = new();
    private string? _docId;
    private Dictionary<string, object?>? _state;

    public bool Persist { get; }

    internal MaskSession(BBAnonym client, AnonymizeOptions options)
    {
        _client = client;
        _options = options;
        Persist = options.Persist;
    }

    public async Task<Tagged> AnonymizeAsync(string text, CancellationToken cancellationToken = default)
    {
        var tagged = await _client.AnonymizeAsync(text, _options, cancellationToken).ConfigureAwait(false);
        return Remember(tagged);
    }

    public Tagged Anonymize(string text) => AnonymizeAsync(text).GetAwaiter().GetResult();

    public async Task<Tagged> AnonymizeFileAsync(string path, CancellationToken cancellationToken = default)
    {
        var tagged = await _client.AnonymizePathAsync(path, _options, cancellationToken).ConfigureAwait(false);
        return Remember(tagged);
    }

    public Tagged AnonymizeFile(string path) => AnonymizeFileAsync(path).GetAwaiter().GetResult();

    public async Task<Tagged> AnonymizeDocumentsAsync(IReadOnlyList<string> paths, CancellationToken cancellationToken = default)
    {
        var tagged = await _client.AnonymizeDocumentsAsync(paths, _options, cancellationToken).ConfigureAwait(false);
        return Remember(tagged);
    }

    public Tagged AnonymizeDocuments(IReadOnlyList<string> paths)
        => AnonymizeDocumentsAsync(paths).GetAwaiter().GetResult();

    public Task<string> RestoreAsync(string modelText, CancellationToken cancellationToken = default)
    {
        if (Persist)
        {
            if (string.IsNullOrEmpty(_docId))
                throw new BBError(400, "Oturumda anonymize yok");
            return _client.RestoreAsync(_docId, modelText, cancellationToken);
        }
        if (_state is null || _state.Count == 0)
            throw new BBError(400, "Oturumda state yok");
        return _client.RestoreAsync(_state, modelText, cancellationToken);
    }

    public string Restore(string modelText) => RestoreAsync(modelText).GetAwaiter().GetResult();

    public async Task CloseAsync(CancellationToken cancellationToken = default)
    {
        if (!Persist)
        {
            _state = null;
            return;
        }
        foreach (var docId in _ids.ToArray())
        {
            try
            {
                await _client.DeleteAsync(docId, cancellationToken).ConfigureAwait(false);
            }
            catch (BBError)
            {
            }
        }
        _ids.Clear();
    }

    public void Close() => CloseAsync().GetAwaiter().GetResult();

    public void Dispose() => Close();

    public async ValueTask DisposeAsync() => await CloseAsync().ConfigureAwait(false);

    private Tagged Remember(Tagged tagged)
    {
        _docId = tagged.DocId;
        _state = tagged.State;
        if (!string.IsNullOrEmpty(tagged.DocId))
            _ids.Add(tagged.DocId);
        return tagged;
    }
}
