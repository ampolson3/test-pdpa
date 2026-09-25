"use client";

import { Fragment, useMemo, useRef, useState, type ReactNode } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import {
  createApiClient,
  fileDownloadHref,
  useActivity,
  useAttachments,
  useCollabMutations,
  useComments,
  useMentionSearch,
  type Comment,
} from "@pdpa/api-client";
import { FileUploader } from "./file-uploader";

const BFF = "/api/bff";
const when: Intl.DateTimeFormatOptions = { month: "short", hour: "2-digit", minute: "2-digit" };

type Tab = "comments" | "attachments" | "activity";

/**
 * Comments, attachments and activity of any record (PLT-07) — every module mounts this same component
 * with its record's type and id; access follows the policy the module registered for that type.
 */
export function RecordCollaboration({ entityType, entityId, canWrite, currentUserId }: {
  entityType: string;
  entityId: string;
  canWrite: boolean;
  currentUserId?: string;
}) {
  const t = useTranslations("collab");
  const [tab, setTab] = useState<Tab>("comments");
  const client = useMemo(() => createApiClient(BFF), []);
  const rec = { entityType, entityId };

  return (
    <section className="rounded-md border border-slate-200 bg-white">
      <div role="tablist" className="flex gap-1 border-b border-slate-200 px-2">
        {(["comments", "attachments", "activity"] as Tab[]).map((k) => (
          <button key={k} role="tab" aria-selected={tab === k} onClick={() => setTab(k)}
            className={`px-3 py-2 text-sm ${tab === k ? "border-b-2 border-slate-900 font-medium" : "text-slate-500"}`}>
            {t(`tab.${k}`)}
          </button>
        ))}
      </div>
      <div className="p-3">
        {tab === "comments" && <Comments client={client} rec={rec} canWrite={canWrite} currentUserId={currentUserId} />}
        {tab === "attachments" && <Attachments client={client} rec={rec} canWrite={canWrite} />}
        {tab === "activity" && <ActivityFeed client={client} rec={rec} />}
      </div>
    </section>
  );
}

type Props = { client: ReturnType<typeof createApiClient>; rec: { entityType: string; entityId: string } };

function Comments({ client, rec, canWrite, currentUserId }: Props & { canWrite: boolean; currentUserId?: string }) {
  const t = useTranslations("collab");
  const locale = useLocale() as Locale;
  const list = useComments(client, rec);
  const m = useCollabMutations(client, rec);
  const [replyTo, setReplyTo] = useState<string>();
  const [editing, setEditing] = useState<Comment>();

  if (list.isPending) return <p className="text-sm text-slate-500">{t("loading")}</p>;
  if (list.isError) return <p className="text-sm text-red-700">{t("loadError")}</p>;
  const roots = list.data.filter((c) => !c.parent_id);
  const replies = (id: string) => list.data.filter((c) => c.parent_id === id);

  const item = (c: Comment, isRoot: boolean) => (
    <div className={`text-sm ${isRoot ? "" : "ml-6 border-l border-slate-200 pl-3"}`}>
      <p className="text-xs text-slate-500">
        <span className="font-medium text-slate-700">{c.author_name || t("unknownUser")}</span> · {formatDate(c.created_at, locale, when)}
      </p>
      {editing?.id === c.id ? (
        <Composer client={client} initial={c.body} submitLabel={t("save")} busy={m.edit.isPending}
          onCancel={() => setEditing(undefined)}
          onSubmit={(body) => m.edit.mutate({ id: c.id, rowVersion: c.row_version, body }, { onSuccess: () => setEditing(undefined) })} />
      ) : (
        <p className="whitespace-pre-wrap">{renderBody(c.body)}</p>
      )}
      {canWrite && editing?.id !== c.id && (
        <div className="mt-1 flex gap-3 text-xs text-slate-500">
          {isRoot && <button onClick={() => setReplyTo(c.id)}>{t("reply")}</button>}
          {isRoot && <button onClick={() => m.resolve.mutate({ id: c.id, resolved: !c.resolved })}>{c.resolved ? t("reopen") : t("resolve")}</button>}
          {c.author_id === currentUserId && <button onClick={() => setEditing(c)}>{t("edit")}</button>}
          {c.author_id === currentUserId && (!isRoot || replies(c.id).length === 0) && (
            <button onClick={() => m.remove.mutate({ id: c.id, rowVersion: c.row_version })}>{t("delete")}</button>
          )}
        </div>
      )}
    </div>
  );

  return (
    <div className="space-y-4">
      {roots.length === 0 && <p className="text-sm text-slate-500">{t("noComments")}</p>}
      {roots.map((c) => (
        <div key={c.id} className={`space-y-2 ${c.resolved ? "opacity-60" : ""}`}>
          {c.resolved && <span className="rounded bg-green-100 px-1.5 text-xs text-green-800">{t("resolved")}</span>}
          {item(c, true)}
          {replies(c.id).map((r) => <Fragment key={r.id}>{item(r, false)}</Fragment>)}
          {replyTo === c.id && (
            <div className="ml-6">
              <Composer client={client} submitLabel={t("reply")} busy={m.add.isPending} onCancel={() => setReplyTo(undefined)}
                onSubmit={(body) => m.add.mutate({ body, parentId: c.id }, { onSuccess: () => setReplyTo(undefined) })} />
            </div>
          )}
        </div>
      ))}
      {canWrite && (
        <Composer client={client} submitLabel={t("post")} busy={m.add.isPending} placeholder={t("placeholder")}
          onSubmit={(body, reset) => m.add.mutate({ body }, { onSuccess: reset })} />
      )}
      {(m.add.isError || m.edit.isError || m.remove.isError) && <p className="text-sm text-red-700">{t("saveError")}</p>}
    </div>
  );
}

/** Mentions are stored as @[Name](user-id); show them as a highlighted @Name. */
function renderBody(body: string): ReactNode[] {
  const out: ReactNode[] = [];
  const re = /@\[([^\]\n]{1,100})\]\([0-9a-fA-F-]{36}\)/g;
  let last = 0;
  for (let m = re.exec(body); m; m = re.exec(body)) {
    out.push(body.slice(last, m.index));
    out.push(<span key={m.index} className="rounded bg-blue-50 px-0.5 text-blue-800">@{m[1]}</span>);
    last = m.index + m[0].length;
  }
  out.push(body.slice(last));
  return out;
}

/** Textarea with an @mention picker: typing "@na" suggests users; picking one inserts @[Name](id). */
function Composer({ client, initial = "", submitLabel, busy, placeholder, onSubmit, onCancel }: {
  client: ReturnType<typeof createApiClient>;
  initial?: string;
  submitLabel: string;
  busy: boolean;
  placeholder?: string;
  onSubmit: (body: string, reset: () => void) => void;
  onCancel?: () => void;
}) {
  const t = useTranslations("collab");
  const [text, setText] = useState(initial);
  const [query, setQuery] = useState<string | null>(null);
  const area = useRef<HTMLTextAreaElement>(null);
  const users = useMentionSearch(client, query);

  const onChange = (value: string) => {
    setText(value);
    const caret = area.current?.selectionStart ?? value.length;
    const m = /(?:^|\s)@([^\s@[\]()]{1,30})$/.exec(value.slice(0, caret));
    setQuery(m ? m[1] : null);
  };
  const pick = (u: { id: string; display_name: string }) => {
    const caret = area.current?.selectionStart ?? text.length;
    const before = text.slice(0, caret).replace(/@([^\s@[\]()]{1,30})$/, `@[${u.display_name}](${u.id}) `);
    setText(before + text.slice(caret));
    setQuery(null);
    area.current?.focus();
  };

  return (
    <div className="relative space-y-2">
      <textarea ref={area} aria-label={t("commentLabel")} className="h-20 w-full rounded-md border border-slate-300 px-2 py-1 text-sm"
        value={text} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} />
      {query && !!users.data?.length && (
        <ul role="listbox" aria-label={t("mentionLabel")} className="absolute z-10 w-64 rounded-md border border-slate-200 bg-white text-sm shadow">
          {users.data.map((u) => (
            <li key={u.id} role="option" aria-selected={false}>
              <button type="button" className="block w-full px-3 py-1.5 text-left hover:bg-slate-50" onClick={() => pick(u)}>{u.display_name}</button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex gap-2">
        <Button disabled={busy || !text.trim()} onClick={() => onSubmit(text, () => setText(""))}>{submitLabel}</Button>
        {onCancel && <Button variant="ghost" onClick={onCancel}>{t("cancel")}</Button>}
      </div>
    </div>
  );
}

function Attachments({ client, rec, canWrite }: Props & { canWrite: boolean }) {
  const t = useTranslations("collab");
  const locale = useLocale() as Locale;
  const list = useAttachments(client, rec);
  const { attach } = useCollabMutations(client, rec);
  return (
    <div className="space-y-3">
      {list.isPending ? (
        <p className="text-sm text-slate-500">{t("loading")}</p>
      ) : !list.data?.length ? (
        <p className="text-sm text-slate-500">{t("noAttachments")}</p>
      ) : (
        <ul className="divide-y divide-slate-100 text-sm">
          {list.data.map((a) => (
            <li key={a.id} className="flex items-center justify-between gap-2 py-2">
              <span className="truncate">{a.file_name}</span>
              <span className="text-xs text-slate-500">{a.uploader_name} · {formatDate(a.created_at, locale, when)}</span>
              {a.av_status === "clean" ? (
                <a className="underline" href={fileDownloadHref(BFF, a.id)}>{t("download")}</a>
              ) : (
                <span className="text-xs text-slate-500">{t(`av.${a.av_status}`)}</span>
              )}
            </li>
          ))}
        </ul>
      )}
      {canWrite && <FileUploader onUploaded={(f) => attach.mutate(f.id)} />}
    </div>
  );
}

function ActivityFeed({ client, rec }: Props) {
  const t = useTranslations("collab");
  const locale = useLocale() as Locale;
  const list = useActivity(client, rec);
  if (list.isPending) return <p className="text-sm text-slate-500">{t("loading")}</p>;
  if (!list.data?.length) return <p className="text-sm text-slate-500">{t("noActivity")}</p>;
  return (
    <ol className="space-y-2 text-sm">
      {list.data.map((a) => (
        <li key={a.id} className="flex gap-2">
          <span className="whitespace-nowrap text-xs text-slate-500">{formatDate(a.occurred_at, locale, when)}</span>
          <span>
            <span className="font-medium">{a.actor_name || t(`actor.${a.actor_type === "system" ? "system" : "unknown"}`)}</span>{" "}
            {t.has(`action.${a.action.replace(/\./g, "_")}`) ? t(`action.${a.action.replace(/\./g, "_")}`) : a.action}
          </span>
        </li>
      ))}
    </ol>
  );
}
