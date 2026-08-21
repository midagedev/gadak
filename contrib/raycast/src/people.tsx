import { useEffect, useState } from "react";
import {
  Action,
  ActionPanel,
  Icon,
  List,
  getPreferenceValues,
  open,
} from "@raycast/api";
import {
  INSTALL_COMMAND,
  INSTALL_GUIDE_URL,
  forgetResolvedGadak,
  gadakErrorTitle,
  isSearchFail,
  personLink,
  resolveGadakBinary,
  runPeople,
  searchErrorDetail,
  searchErrorFull,
  type Person,
  type SearchFail,
} from "./gadak";

type Prefs = {
  gadakPath?: string;
  profile?: string;
};

function MissingBinaryView() {
  return (
    <List.EmptyView
      icon={Icon.Download}
      title="gadak is not installed"
      description="Install gadak, then try again. Or set the gadak binary preference."
      actions={
        <ActionPanel>
          <Action.CopyToClipboard
            title="Copy Install Command"
            icon={Icon.Clipboard}
            content={INSTALL_COMMAND}
          />
          <Action.OpenInBrowser
            title="Open Install Guide"
            icon={Icon.Globe}
            url={INSTALL_GUIDE_URL}
          />
        </ActionPanel>
      }
    />
  );
}

function CliErrorView({ fail }: { fail: SearchFail }) {
  return (
    <List.EmptyView
      icon={Icon.Warning}
      title={gadakErrorTitle(fail, "gadak sql failed")}
      description={searchErrorDetail(fail)}
      actions={
        <ActionPanel>
          <Action.CopyToClipboard
            title="Copy Full Error"
            icon={Icon.Clipboard}
            content={searchErrorFull(fail)}
          />
        </ActionPanel>
      }
    />
  );
}

export default function Command() {
  const prefs = getPreferenceValues<Prefs>();
  const bin = resolveGadakBinary(prefs.gadakPath);
  const profile = prefs.profile?.trim() ?? "";

  const [people, setPeople] = useState<Person[]>([]);
  const [loading, setLoading] = useState(Boolean(bin));
  const [error, setError] = useState<SearchFail | null>(null);

  useEffect(() => {
    if (!bin) return;
    let live = true;
    setLoading(true);
    runPeople(bin, profile)
      .then((rows) => {
        if (!live) return;
        setPeople(rows);
        setError(null);
      })
      .catch((e) => {
        if (!live) return;
        const fail: SearchFail = isSearchFail(e)
          ? e
          : { stderr: "", message: e instanceof Error ? e.message : String(e) };
        if (fail.code === "ENOENT") forgetResolvedGadak();
        setPeople([]);
        setError(fail);
      })
      .finally(() => {
        if (live) setLoading(false);
      });
    return () => {
      live = false;
    };
  }, [bin, profile]);

  return (
    <List
      isLoading={loading}
      searchBarPlaceholder="Filter people from the local gadak mirror…"
    >
      {!bin ? (
        <MissingBinaryView />
      ) : error ? (
        <CliErrorView fail={error} />
      ) : people.length === 0 && !loading ? (
        <List.EmptyView
          icon={Icon.Person}
          title="No people in this mirror"
          description="No assignees or reporters with an account id or email."
        />
      ) : (
        people.map((p) => {
          const link = personLink(p.identity, profile);
          return (
            <List.Item
              key={p.identity}
              icon={Icon.Person}
              title={p.name}
              subtitle={p.identity === p.name ? undefined : p.identity}
              keywords={[p.email, p.identity].filter(Boolean)}
              actions={
                <ActionPanel>
                  {link ? (
                    <Action
                      title="Open in Gadak"
                      icon={Icon.ArrowRight}
                      onAction={() => open(link)}
                    />
                  ) : null}
                  {link ? (
                    <Action.CopyToClipboard
                      title="Copy Deep Link"
                      icon={Icon.Link}
                      content={link}
                    />
                  ) : null}
                </ActionPanel>
              }
            />
          );
        })
      )}
    </List>
  );
}
