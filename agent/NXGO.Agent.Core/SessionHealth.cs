namespace NXGO.Agent.Core;

public enum SessionHealth
{
    Healthy = 0,
    Suspect = 1,
    Poisoned = 2,
    Lost = 3,
}

public sealed class SessionHealthState
{
    private SessionHealth _value = SessionHealth.Healthy;

    public SessionHealth Value => _value;

    public bool IsReusable => _value == SessionHealth.Healthy;

    public void MarkSuspect()
    {
        if (_value == SessionHealth.Healthy)
        {
            _value = SessionHealth.Suspect;
        }
    }

    public void MarkPoisoned() => _value = SessionHealth.Poisoned;

    public void MarkLost() => _value = SessionHealth.Lost;

    public void RequireReusable()
    {
        if (!IsReusable)
        {
            throw new InvalidOperationException($"NX session is not reusable: {_value}");
        }
    }
}
