using System.Net;
using System.Net.Sockets;
using System.Text;
using System.Text.Json;
using Xunit;

[assembly: CollectionBehavior(DisableTestParallelization = true)]

namespace BigBrains.Tests;

public class ClientTests
{
    private static Dictionary<string, object?> TaggedBody(Dictionary<string, object?>? extra = null)
    {
        var body = new Dictionary<string, object?>
        {
            ["doc_id"] = "doc-1",
            ["text"] = "Davacı <PERSON_0>",
            ["entities_found"] = new Dictionary<string, int> { ["PERSON"] = 1 },
            ["balance_credits"] = "9.00",
            ["state"] = null,
            ["placeholder_style"] = "angle",
            ["keep_legal_form"] = false,
            ["health_data"] = false,
            ["llm_review"] = null,
        };
        if (extra is null)
            return body;
        foreach (var pair in extra)
            body[pair.Key] = pair.Value;
        return body;
    }

    private static BBError Require(Action action, int status)
    {
        var error = Assert.Throws<BBError>(action);
        Assert.Equal(status, error.StatusCode);
        return error;
    }

    [Fact]
    public async Task AnonymizeSendsWireBody()
    {
        Incoming? seen = null;
        using var server = new TestServer
        {
            Handle = (incoming, _) =>
            {
                seen = incoming;
                return Task.FromResult(Outgoing.Json(200, TaggedBody()));
            },
        };
        using var client = new BBAnonym(server.Url + "/", "dm_test_abc");
        var tagged = client.Anonymize("Davacı Hasan Demirkol", new AnonymizeOptions
        {
            PlaceholderStyle = BbMask.StyleBracketTr,
            KeepLegalForm = true,
            HealthData = true,
            LlmReview = true,
        });
        var again = await client.AnonymizeAsync("Davacı Hasan Demirkol", new AnonymizeOptions
        {
            PlaceholderStyle = BbMask.StyleBracketTr,
            KeepLegalForm = true,
            HealthData = true,
            LlmReview = true,
        });

        Assert.NotNull(seen);
        Assert.Equal("POST", seen!.Method);
        Assert.Equal("/v1/anonymize", seen.Target);
        Assert.Equal("Bearer dm_test_abc", seen.Authorization);
        using var doc = JsonDocument.Parse(seen.Body);
        Assert.Equal("Davacı Hasan Demirkol", doc.RootElement.GetProperty("text").GetString());
        Assert.True(doc.RootElement.GetProperty("persist").GetBoolean());
        Assert.Equal("bracket_tr", doc.RootElement.GetProperty("placeholder_style").GetString());
        Assert.True(doc.RootElement.GetProperty("keep_legal_form").GetBoolean());
        Assert.True(doc.RootElement.GetProperty("health_data").GetBoolean());
        Assert.True(doc.RootElement.GetProperty("llm_review").GetBoolean());
        Assert.Equal("Davacı <PERSON_0>", tagged.Text);
        Assert.Equal("doc-1", tagged.DocId);
        Assert.Null(tagged.State);
        Assert.Equal(1, tagged.EntitiesFound["PERSON"]);
        Assert.Equal("9.00", tagged.BalanceCredits);
        Assert.Equal("req-1", tagged.RequestId);
        Assert.Equal("angle", tagged.PlaceholderStyle);
        Assert.False(tagged.KeepLegalForm);
        Assert.False(tagged.HealthData);
        Assert.Null(tagged.LlmReview);
        Assert.Equal(tagged.DocId, again.DocId);
    }

    [Fact]
    public void PersistFalseDropsDocId()
    {
        using var server = new TestServer
        {
            Handle = (incoming, _) =>
            {
                using var doc = JsonDocument.Parse(incoming.Body);
                Assert.False(doc.RootElement.GetProperty("persist").GetBoolean());
                return Task.FromResult(Outgoing.Json(200, TaggedBody(new Dictionary<string, object?>
                {
                    ["state"] = new Dictionary<string, object?>
                    {
                        ["PERSON"] = new Dictionary<string, object?> { ["Ali"] = "<PERSON_0>" },
                    },
                })));
            },
        };
        var tagged = new BBAnonym(server.Url, "dm_test_abc").Anonymize("Ali", new AnonymizeOptions { Persist = false });
        Assert.Null(tagged.DocId);
        var person = Assert.IsType<Dictionary<string, object?>>(tagged.State!["PERSON"]);
        Assert.Equal("<PERSON_0>", person["Ali"]);
    }

    [Fact]
    public void InvalidPlaceholderStyleFailsBeforeRequest()
    {
        var calls = 0;
        using var server = new TestServer
        {
            Handle = (_, _) =>
            {
                calls++;
                return Task.FromResult(Outgoing.Json(200, TaggedBody()));
            },
        };
        var error = Require(() => new BBAnonym(server.Url, "dm_test_abc").Anonymize("Ali", new AnonymizeOptions
        {
            PlaceholderStyle = "stars",
        }), 400);
        Assert.Contains("placeholder_style", error.Detail);
        Assert.Equal(0, calls);
    }

    [Fact]
    public void RestoreSendsDocIdOrState()
    {
        var bodies = new List<string>();
        using var server = new TestServer
        {
            Handle = (incoming, _) =>
            {
                bodies.Add(incoming.Body);
                return Task.FromResult(Outgoing.Json(200, new Dictionary<string, object?> { ["text"] = "açık metin" }));
            },
        };
        var client = new BBAnonym(server.Url, "dm_test_abc");
        Assert.Equal("açık metin", client.Restore("doc-1", "<PERSON_0>"));
        Assert.Equal("açık metin", client.Restore(new Dictionary<string, object?>
        {
            ["PERSON"] = new Dictionary<string, object?>(),
        }, "<PERSON_0>"));
        using (var first = JsonDocument.Parse(bodies[0]))
        {
            Assert.Equal("<PERSON_0>", first.RootElement.GetProperty("text").GetString());
            Assert.Equal("doc-1", first.RootElement.GetProperty("doc_id").GetString());
        }
        using (var second = JsonDocument.Parse(bodies[1]))
        {
            Assert.Equal("<PERSON_0>", second.RootElement.GetProperty("text").GetString());
            Assert.Equal(JsonValueKind.Object, second.RootElement.GetProperty("state").ValueKind);
        }
        Require(() => client.Restore(12, "x"), 400);
    }

    [Fact]
    public void GetMapEncodesIdAndDeleteReadsDeleted()
    {
        var seen = new List<string>();
        using var server = new TestServer
        {
            Handle = (incoming, _) =>
            {
                seen.Add(incoming.Method + " " + incoming.Target);
                if (incoming.Method == "GET")
                {
                    return Task.FromResult(Outgoing.Json(200, new Dictionary<string, object?>
                    {
                        ["state"] = new Dictionary<string, object?>
                        {
                            ["PERSON"] = new Dictionary<string, object?> { ["Ali"] = "<PERSON_0>" },
                        },
                    }));
                }
                return Task.FromResult(Outgoing.Json(200, new Dictionary<string, object?> { ["deleted"] = true }));
            },
        };
        var client = new BBAnonym(server.Url, "dm_test_abc");
        var state = client.GetMap("a/b c", true);
        var person = Assert.IsType<Dictionary<string, object?>>(state["PERSON"]);
        Assert.Equal("<PERSON_0>", person["Ali"]);
        Assert.True(client.Delete("a/b c"));
        Assert.Equal("GET /v1/mappings/a%2Fb%20c?once=true", seen[0]);
        Assert.Equal("DELETE /v1/mappings/a%2Fb%20c", seen[1]);
    }

    [Fact]
    public void FileUploadUsesMultipartFieldNames()
    {
        var path = Path.Combine(Path.GetTempPath(), "bb-mask-" + Guid.NewGuid().ToString("N"), "dilekce.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        File.WriteAllText(path, "Davacı Ali");
        try
        {
            string body = "";
            using var server = new TestServer
            {
                Handle = (incoming, _) =>
                {
                    body = incoming.Body;
                    Assert.Equal("/v1/anonymize/file", incoming.Target);
                    return Task.FromResult(Outgoing.Json(200, TaggedBody(new Dictionary<string, object?>
                    {
                        ["doc_id"] = "should-drop",
                    })));
                },
            };
            var tagged = new BBAnonym(server.Url, "dm_test_abc").AnonymizeFile(path, new AnonymizeOptions { Persist = false });
            Assert.Null(tagged.DocId);
            Assert.True(HasField(body, "persist"), body);
            Assert.Contains("false", body);
            Assert.True(HasField(body, "placeholder_style"), body);
            Assert.Contains("angle", body);
            Assert.True(HasField(body, "file"), body);
            Assert.Contains("dilekce.txt", body);
            Assert.Contains("Davacı Ali", body);
        }
        finally
        {
            Directory.Delete(Path.GetDirectoryName(path)!, true);
        }
    }

    [Fact]
    public void DocumentsCount()
    {
        var dir = Path.Combine(Path.GetTempPath(), "bb-mask-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(dir);
        var first = Path.Combine(dir, "a.txt");
        var second = Path.Combine(dir, "b.txt");
        File.WriteAllText(first, "bir");
        File.WriteAllText(second, "iki");
        try
        {
            var calls = 0;
            using var server = new TestServer
            {
                Handle = (incoming, _) =>
                {
                    calls++;
                    Assert.Equal("/v1/anonymize/documents", incoming.Target);
                    Assert.Equal(2, CountFields(incoming.Body, "files"));
                    Assert.Contains("a.txt", incoming.Body);
                    Assert.Contains("b.txt", incoming.Body);
                    return Task.FromResult(Outgoing.Json(200, TaggedBody()));
                },
            };
            var client = new BBAnonym(server.Url, "dm_test_abc");
            var tagged = client.AnonymizeDocuments(new[] { first, second });
            Assert.Equal("doc-1", tagged.DocId);
            Require(() => client.AnonymizeDocuments(Array.Empty<string>()), 400);
            var error = Require(() => client.AnonymizeDocuments(new[] { "1", "2", "3", "4" }, new AnonymizeOptions
            {
                PlaceholderStyle = "nope",
            }), 400);
            Assert.Contains("placeholder_style", error.Detail);
            Assert.Equal(1, calls);
        }
        finally
        {
            Directory.Delete(dir, true);
        }
    }

    [Fact]
    public void HttpErrorKeepsStatusDetailAndRequestId()
    {
        using var server = new TestServer
        {
            Handle = (_, _) => Task.FromResult(Outgoing.Json(402, new Dictionary<string, string> { ["detail"] = "Bakiye yok" }, "req-402")),
        };
        var error = Require(() => new BBAnonym(server.Url, "dm_test_abc").Anonymize("Ali"), 402);
        Assert.Equal("Bakiye yok", error.Detail);
        Assert.Equal("req-402", error.RequestId);
        Assert.Equal("Bakiye yok", error.Message);
    }

    [Fact]
    public void NonJsonSuccessIs502()
    {
        using var server = new TestServer
        {
            Handle = (_, _) => Task.FromResult(Outgoing.Text(200, "tamam", "req-bad")),
        };
        var error = Require(() => new BBAnonym(server.Url, "dm_test_abc").Anonymize("Ali"), 502);
        Assert.Equal("req-bad", error.RequestId);
        Assert.Equal("Servis JSON dönmedi", error.Detail);
    }

    [Fact]
    public void TimeoutIs504AndClosedPortIs503()
    {
        using var hanging = new TestServer
        {
            Handle = async (_, ct) =>
            {
                await Task.Delay(Timeout.Infinite, ct);
                return Outgoing.Text(200, "", "req-1");
            },
        };
        Require(() => new BBAnonym(hanging.Url, "dm_test_abc", TimeSpan.FromMilliseconds(400)).Anonymize("Ali"), 504);

        var listener = new TcpListener(IPAddress.Loopback, 0);
        listener.Start();
        var port = ((IPEndPoint)listener.LocalEndpoint).Port;
        listener.Stop();
        Require(() => new BBAnonym($"http://127.0.0.1:{port}", "dm_test_abc", TimeSpan.FromSeconds(1)).Anonymize("Ali"), 503);
    }

    [Fact]
    public void AnonymizeBytesSendsNameAndContent()
    {
        string body = "";
        using var server = new TestServer
        {
            Handle = (incoming, _) =>
            {
                body = incoming.Body;
                Assert.Equal("/v1/anonymize/file", incoming.Target);
                return Task.FromResult(Outgoing.Json(200, TaggedBody()));
            },
        };
        var tagged = new BBAnonym(server.Url, "dm_test_abc").AnonymizeBytes("ek.pdf", Encoding.ASCII.GetBytes("%PDF"));
        Assert.Equal("doc-1", tagged.DocId);
        Assert.Contains("ek.pdf", body);
        Assert.Contains("%PDF", body);
    }

    [Fact]
    public void TokenIsNotPrinted()
    {
        var client = new BBAnonym("http://127.0.0.1:9", "dm_test_secret_value");
        Assert.DoesNotContain("dm_test_secret_value", client.ToString());
    }

    [Fact]
    public void SessionDeletesIdsAndHidesState()
    {
        var calls = new List<string>();
        using var server = new TestServer
        {
            Handle = (incoming, _) =>
            {
                calls.Add(incoming.Method + " " + incoming.Target);
                if (incoming.Method == "DELETE")
                {
                    if (incoming.Target.EndsWith("id-2", StringComparison.Ordinal))
                        return Task.FromResult(Outgoing.Json(404, new Dictionary<string, string> { ["detail"] = "yok" }));
                    return Task.FromResult(Outgoing.Json(200, new Dictionary<string, object?> { ["deleted"] = true }));
                }
                if (incoming.Target == "/v1/restore")
                    return Task.FromResult(Outgoing.Json(200, new Dictionary<string, object?> { ["text"] = "<PERSON_0>" }));
                using var doc = JsonDocument.Parse(incoming.Body);
                var persist = doc.RootElement.GetProperty("persist").GetBoolean();
                var body = TaggedBody(new Dictionary<string, object?> { ["text"] = "<PERSON_0>" });
                if (persist)
                {
                    body["doc_id"] = "id-" + calls.Count;
                    body["state"] = null;
                }
                else
                {
                    body["doc_id"] = "should-drop";
                    body["state"] = new Dictionary<string, object?>
                    {
                        ["PERSON"] = new Dictionary<string, object?> { ["Ali"] = "<PERSON_0>" },
                    };
                }
                return Task.FromResult(Outgoing.Json(200, body));
            },
        };

        BbMask.Init("dm_test_abc", server.Url);
        var open = BbMask.Session();
        var first = open.Anonymize("Ali");
        var second = open.Anonymize("Veli");
        Assert.Equal("<PERSON_0>", open.Restore("cevap"));
        open.Close();
        Assert.Contains("DELETE /v1/mappings/" + first.DocId, calls);
        Assert.Contains("DELETE /v1/mappings/" + second.DocId, calls);

        var local = BbMask.Session(new AnonymizeOptions
        {
            Persist = false,
            PlaceholderStyle = BbMask.StyleBracketTr,
        });
        var tagged = local.Anonymize("Ali");
        Assert.Null(tagged.DocId);
        Assert.Equal("<PERSON_0>", local.Restore("<PERSON_0>"));
        var before = calls.Count;
        local.Close();
        Assert.Equal(before, calls.Count);
        Require(() => local.Restore("tekrar"), 400);

        var empty = BbMask.Session();
        Require(() => empty.Restore("yok"), 400);
        var error = Require(() => BbMask.Session(new AnonymizeOptions { PlaceholderStyle = "nope" }), 400);
        Assert.Contains("placeholder_style", error.Detail);
    }

    [Fact]
    public void ModuleCallsFollowInit()
    {
        var dir = Path.Combine(Path.GetTempPath(), "bb-mask-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(dir);
        var path = Path.Combine(dir, "ek.txt");
        File.WriteAllText(path, "ek");
        try
        {
            var seen = new List<string>();
            string? auth = null;
            using var server = new TestServer
            {
                Handle = (incoming, _) =>
                {
                    seen.Add(incoming.Target);
                    auth ??= incoming.Authorization;
                    if (incoming.Method == "POST" && incoming.Target == "/v1/restore")
                        return Task.FromResult(Outgoing.Json(200, new Dictionary<string, object?> { ["text"] = "ok" }));
                    return Task.FromResult(Outgoing.Json(200, TaggedBody()));
                },
            };
            Require(() => BbMask.Init("  "), 401);
            BbMask.Init("  dm_test_abc  ", server.Url);
            var tagged = BbMask.Anonymize("Ali");
            Assert.Equal("doc-1", tagged.DocId);
            Assert.Equal("ok", BbMask.Restore(tagged.DocId, "cevap"));
            Assert.Empty(BbMask.GetMap(tagged.DocId!, false));
            Assert.False(BbMask.Delete(tagged.DocId!));
            BbMask.AnonymizeFile(path);
            BbMask.AnonymizeBytes("ek.txt", Encoding.UTF8.GetBytes("ek"));
            BbMask.AnonymizeDocuments(new[] { path });
            Assert.Equal("Bearer dm_test_abc", auth);
            Assert.Equal(new[]
            {
                "/v1/anonymize",
                "/v1/restore",
                "/v1/mappings/doc-1?once=false",
                "/v1/mappings/doc-1",
                "/v1/anonymize/file",
                "/v1/anonymize/file",
                "/v1/anonymize/documents",
            }, seen);
        }
        finally
        {
            Directory.Delete(dir, true);
        }
    }

    [Fact]
    public void ModuleCallsBeforeInit()
    {
        BbMask.Reset();
        Require(() => BbMask.Anonymize("Ali"), 401);
        Require(() => BbMask.AnonymizeBytes("a.txt", Encoding.UTF8.GetBytes("a")), 401);
        Require(() => BbMask.Session(), 401);
    }

    private static bool HasField(string body, string name)
        => body.Contains($"name=\"{name}\"", StringComparison.Ordinal)
           || body.Contains($"name={name}", StringComparison.Ordinal);

    private static int CountFields(string body, string name)
        => Count(body, $"name=\"{name}\"") + Count(body, $"name={name}");

    private static int Count(string text, string token)
    {
        var count = 0;
        var index = 0;
        while ((index = text.IndexOf(token, index, StringComparison.Ordinal)) >= 0)
        {
            count++;
            index += token.Length;
        }
        return count;
    }
}
