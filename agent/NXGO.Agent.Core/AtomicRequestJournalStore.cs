using System;
using System.IO;

namespace NXGO.Agent.Core;

/// <summary>Crash-safe file boundary for the bounded request journal.</summary>
public sealed class AtomicRequestJournalStore
{
    public AtomicRequestJournalStore(string path)
    {
        if (string.IsNullOrWhiteSpace(path)) throw new ArgumentException("path is required", nameof(path));
        Path = path;
    }

    public string Path { get; }

    public void Save(RequestJournal journal)
    {
        if (journal is null) throw new ArgumentNullException(nameof(journal));
        var directory = System.IO.Path.GetDirectoryName(System.IO.Path.GetFullPath(Path));
        if (!string.IsNullOrEmpty(directory)) Directory.CreateDirectory(directory);
        var temp = Path + ".tmp-" + Guid.NewGuid().ToString("N");
        try
        {
            // WriteThrough plus Flush(true) makes the complete snapshot durable
            // before the rename publishes it. A partially written primary file
            // is therefore never the result of this store's own Save operation.
            using (var stream = new FileStream(temp, FileMode.CreateNew, FileAccess.Write, FileShare.None, 4096, FileOptions.WriteThrough))
            {
                journal.SaveSnapshot(stream);
                stream.Flush(true);
            }
            if (File.Exists(Path)) File.Replace(temp, Path, null);
            else File.Move(temp, Path);
        }
        finally
        {
            if (File.Exists(temp)) File.Delete(temp);
        }
    }

    public RequestJournal Load(int capacity = RequestJournal.DefaultCapacity)
    {
        using (var stream = new FileStream(Path, FileMode.Open, FileAccess.Read, FileShare.Read))
        {
            return RequestJournal.LoadSnapshot(stream, capacity);
        }
    }
}
