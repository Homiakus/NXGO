using NXGO.Agent.Core;
using Xunit;

namespace NXGO.Agent.Core.Tests;

public sealed class HandleRegistryTests
{
    [Fact]
    public void Released_slot_is_reused_with_new_generation_and_old_handle_stays_stale()
    {
        var registry = new HandleRegistry<object>("session-a", 7, capacity: 2);
        var firstObject = new object();
        var first = registry.Register(firstObject, "Body", "scope-a");

        Assert.Equal("obj-1", first.ObjectId);
        Assert.Equal((uint)1, first.Generation);
        Assert.Same(firstObject, registry.Resolve(first, "Body"));
        Assert.True(registry.Release(first));

        var secondObject = new object();
        var second = registry.Register(secondObject, "Body", "scope-b");
        Assert.Equal(first.ObjectId, second.ObjectId);
        Assert.Equal((uint)2, second.Generation);
        Assert.Same(secondObject, registry.Resolve(second, "Body"));

        var stale = Assert.Throws<StaleObjectHandleException>(() => registry.Resolve(first, "Body"));
        Assert.Contains("stale object generation", stale.Message);
    }

    [Fact]
    public void Foreign_session_epoch_and_kind_fail_closed()
    {
        var registry = new HandleRegistry<object>("session-a", 5);
        var token = registry.Register(new object(), "Part");

        var foreignSession = Clone(token);
        foreignSession.SessionId = "session-b";
        Assert.Throws<StaleObjectHandleException>(() => registry.Resolve(foreignSession, "Part"));

        var foreignEpoch = Clone(token);
        foreignEpoch.Epoch = 6;
        Assert.Throws<StaleObjectHandleException>(() => registry.Resolve(foreignEpoch, "Part"));

        var forgedKind = Clone(token);
        forgedKind.Kind = "Body";
        Assert.Throws<StaleObjectHandleException>(() => registry.Resolve(forgedKind, "Part"));

        Assert.Throws<StaleObjectHandleException>(() => registry.Resolve(token, "Body"));
        Assert.Equal(1, registry.Count);
    }

    [Fact]
    public void Capacity_is_bounded_and_high_watermark_is_retained()
    {
        var registry = new HandleRegistry<object>("session-a", 1, capacity: 2);
        var one = registry.Register(new object(), "Body");
        var two = registry.Register(new object(), "Body");

        Assert.Equal(2, registry.Count);
        Assert.Equal(2, registry.HighWatermark);
        var ex = Assert.Throws<HandleRegistryCapacityException>(() => registry.Register(new object(), "Body"));
        Assert.Equal(2, ex.Capacity);

        registry.Release(one);
        Assert.Equal(1, registry.Count);
        var three = registry.Register(new object(), "Body");
        Assert.Equal(one.ObjectId, three.ObjectId);
        Assert.Equal(one.Generation + 1, three.Generation);
        Assert.Equal(2, registry.HighWatermark);
        Assert.Same(registry.Resolve(two, "Body"), registry.Resolve(two, "Body"));
    }

    [Fact]
    public void Lease_scope_release_invalidates_only_scope_members_and_reuse_increments_generation()
    {
        var registry = new HandleRegistry<object>("session-a", 9, capacity: 4);
        var a1 = registry.Register(new object(), "Body", "request-a");
        var a2 = registry.Register(new object(), "Feature", "request-a");
        var b1 = registry.Register(new object(), "Body", "request-b");

        Assert.Equal(2, registry.ReleaseScope("request-a"));
        Assert.Equal(1, registry.Count);
        Assert.Throws<StaleObjectHandleException>(() => registry.Resolve(a1, "Body"));
        Assert.Throws<StaleObjectHandleException>(() => registry.Resolve(a2, "Feature"));
        Assert.NotNull(registry.Resolve(b1, "Body"));

        var reused = registry.Register(new object(), "Body", "request-c");
        Assert.True(reused.ObjectId == a1.ObjectId || reused.ObjectId == a2.ObjectId);
        if (reused.ObjectId == a1.ObjectId)
        {
            Assert.Equal(a1.Generation + 1, reused.Generation);
        }
        else
        {
            Assert.Equal(a2.Generation + 1, reused.Generation);
        }
    }

    [Fact]
    public void Missing_generation_cannot_release_a_live_object()
    {
        var registry = new HandleRegistry<object>("session-a", 1);
        var live = registry.Register(new object(), "Body");
        var forged = Clone(live);
        forged.Generation = 0;

        Assert.Throws<StaleObjectHandleException>(() => registry.Release(forged));
        Assert.Equal(1, registry.Count);
        Assert.NotNull(registry.Resolve(live, "Body"));
    }

    private static ObjectHandleToken Clone(ObjectHandleToken token)
    {
        return new ObjectHandleToken
        {
            SessionId = token.SessionId,
            Epoch = token.Epoch,
            ObjectId = token.ObjectId,
            Generation = token.Generation,
            Kind = token.Kind,
            LeaseScopeId = token.LeaseScopeId,
        };
    }
}
