using System.Globalization;
using System.Net;
using System.Net.Sockets;
using System.Text;

namespace BigBrains.Tests;

internal sealed class Incoming
{
    public string Method { get; init; } = "";
    public string Target { get; init; } = "";
    public string Body { get; init; } = "";
    public string Authorization { get; init; } = "";
}

internal sealed class Outgoing
{
    public int Status { get; init; } = 200;
    public byte[] Body { get; init; } = Array.Empty<byte>();
    public string RequestId { get; init; } = "req-1";

    public static Outgoing Json(int status, object body, string requestId = "req-1") => new()
    {
        Status = status,
        Body = System.Text.Json.JsonSerializer.SerializeToUtf8Bytes(body),
        RequestId = requestId,
    };

    public static Outgoing Text(int status, string body, string requestId) => new()
    {
        Status = status,
        Body = Encoding.UTF8.GetBytes(body),
        RequestId = requestId,
    };
}

internal sealed class TestServer : IDisposable
{
    private readonly TcpListener _listener;
    private readonly CancellationTokenSource _cts = new();
    private readonly List<Task> _connections = new();
    private readonly object _gate = new();

    public string Url { get; }
    public Func<Incoming, CancellationToken, Task<Outgoing>> Handle { get; set; }
        = (_, _) => Task.FromResult(Outgoing.Text(500, "handler yok", "req-1"));

    public TestServer()
    {
        _listener = new TcpListener(IPAddress.Loopback, 0);
        _listener.Start();
        var port = ((IPEndPoint)_listener.LocalEndpoint).Port;
        Url = $"http://127.0.0.1:{port}";
        _ = Task.Run(AcceptLoop);
    }

    public void Dispose()
    {
        _cts.Cancel();
        _listener.Stop();
        Task[] pending;
        lock (_gate)
            pending = _connections.ToArray();
        try
        {
            Task.WaitAll(pending, TimeSpan.FromSeconds(2));
        }
        catch (AggregateException)
        {
        }
    }

    private async Task AcceptLoop()
    {
        while (!_cts.IsCancellationRequested)
        {
            TcpClient client;
            try
            {
                client = await _listener.AcceptTcpClientAsync(_cts.Token);
            }
            catch
            {
                break;
            }
            var task = Task.Run(() => Serve(client));
            lock (_gate)
                _connections.Add(task);
        }
    }

    private async Task Serve(TcpClient client)
    {
        using (client)
        using (var stream = client.GetStream())
        {
            var header = await ReadHeaders(stream, _cts.Token);
            var headers = ParseHeaders(header.Text);
            var count = 0;
            if (headers.TryGetValue("content-length", out var lengthText))
                count = int.Parse(lengthText, CultureInfo.InvariantCulture);
            var body = count == 0 ? Array.Empty<byte>() : await ReadCount(stream, header.Rest, count, _cts.Token);
            var incoming = new Incoming
            {
                Method = header.Method,
                Target = header.Target,
                Body = Encoding.UTF8.GetString(body),
                Authorization = headers.GetValueOrDefault("authorization") ?? "",
            };
            Outgoing outgoing;
            try
            {
                outgoing = await Handle(incoming, _cts.Token);
            }
            catch (OperationCanceledException)
            {
                return;
            }
            catch (Exception ex)
            {
                outgoing = Outgoing.Text(500, ex.ToString(), "req-1");
            }
            var payload = Encoding.ASCII.GetBytes(
                $"HTTP/1.1 {outgoing.Status} OK\r\n" +
                "Content-Type: application/json\r\n" +
                $"Content-Length: {outgoing.Body.Length}\r\n" +
                $"X-Request-Id: {outgoing.RequestId}\r\n" +
                "Connection: close\r\n\r\n");
            await stream.WriteAsync(payload, _cts.Token);
            if (outgoing.Body.Length > 0)
                await stream.WriteAsync(outgoing.Body, _cts.Token);
        }
    }

    private static async Task<HeaderRead> ReadHeaders(NetworkStream stream, CancellationToken ct)
    {
        var data = new MemoryStream();
        var buffer = new byte[2048];
        while (true)
        {
            var read = await stream.ReadAsync(buffer, ct);
            if (read == 0)
                throw new IOException("başlık gelmedi");
            data.Write(buffer, 0, read);
            var bytes = data.ToArray();
            for (var i = 0; i <= bytes.Length - 4; i++)
            {
                if (bytes[i] == '\r' && bytes[i + 1] == '\n' && bytes[i + 2] == '\r' && bytes[i + 3] == '\n')
                {
                    var text = Encoding.ASCII.GetString(bytes, 0, i);
                    var rest = bytes[(i + 4)..];
                    var line = text.Split("\r\n", 2)[0];
                    var parts = line.Split(' ');
                    return new HeaderRead(parts[0], parts.Length > 1 ? parts[1] : "/", text, rest);
                }
            }
            if (bytes.Length > 1024 * 1024)
                throw new IOException("başlık çok büyük");
        }
    }

    private static Dictionary<string, string> ParseHeaders(string text)
    {
        var headers = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        foreach (var line in text.Split("\r\n").Skip(1))
        {
            var colon = line.IndexOf(':');
            if (colon <= 0)
                continue;
            headers[line[..colon].Trim()] = line[(colon + 1)..].Trim();
        }
        return headers;
    }

    private static async Task<byte[]> ReadCount(NetworkStream stream, byte[] prefetched, int count, CancellationToken ct)
    {
        var result = new byte[count];
        var offset = Math.Min(prefetched.Length, count);
        if (offset > 0)
            prefetched.AsSpan(0, offset).CopyTo(result);
        while (offset < count)
        {
            var read = await stream.ReadAsync(result.AsMemory(offset, count - offset), ct);
            if (read == 0)
                throw new IOException("gövde eksik");
            offset += read;
        }
        return result;
    }

    private readonly record struct HeaderRead(string Method, string Target, string Text, byte[] Rest);
}
