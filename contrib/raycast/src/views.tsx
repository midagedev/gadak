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
  resolveGadakBinary,
  runViews,
  searchErrorDetail,
  searchErrorFull,
  viewLink,
  viewPartialTooltip,
  type ListedView,
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
      title={gadakErrorTitle(fail, "gadak views failed")}
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

  const [views, setViews] = useState<ListedView[]>([]);
  const [loading, setLoading] = useState(Boolean(bin));
  const [error, setError] = useState<SearchFail | null>(null);

  useEffect(() => {
    if (!bin) return;
    let live = true;
    setLoading(true);
    runViews(bin, profile)
      .then((rows) => {
        if (!live) return;
        setViews(rows);
        setError(null);
      })
      .catch((e) => {
        if (!live) return;
        const fail: SearchFail = isSearchFail(e)
          ? e
          : { stderr: "", message: e instanceof Error ? e.message : String(e) };
        if (fail.code === "ENOENT") forgetResolvedGadak();
        setViews([]);
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
      searchBarPlaceholder="Filter views from the local gadak mirror…"
    >
      {!bin ? (
        <MissingBinaryView />
      ) : error ? (
        <CliErrorView fail={error} />
      ) : views.length === 0 && !loading ? (
        <List.EmptyView
          icon={Icon.List}
          title="No views in this mirror"
          description="Sync to import owned or starred Jira filters, or save one with gadak views save."
        />
      ) : (
        views.map((v) => {
          const hash = v.hash ?? "";
          const jql = v.jql ?? "";
          const unsupported = v.unsupported ?? [];
          const link = viewLink(hash, profile);
          return (
            <List.Item
              key={v.id || v.name}
              icon={Icon.List}
              title={v.name || v.id}
              subtitle={jql || undefined}
              keywords={[jql, v.kind, v.id].filter(Boolean)}
              accessories={
                unsupported.length > 0
                  ? [
                      {
                        icon: Icon.Warning,
                        tooltip: viewPartialTooltip(unsupported),
                      },
                    ]
                  : undefined
              }
              actions={
                <ActionPanel>
                  {link ? (
                    <Action
                      title="Open in Gadak"
                      icon={Icon.ArrowRight}
                      onAction={() => open(link)}
                    />
                  ) : null}
                  {jql ? (
                    <Action.CopyToClipboard
                      title="Copy JQL"
                      icon={Icon.Clipboard}
                      content={jql}
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
