import {
  FileText,
  FolderOpen,
  ImagePlus,
  Wand2,
  Heart,
  HeartOff,
  ListEnd,
  ListPlus,
  ListStart,
  Pencil,
  Play,
  type LucideIcon,
} from "lucide-react";
import { createElement } from "react";
import { useNavigate } from "react-router";
import { errorMessage, unwrap } from "../../api/client";
import type { MoreMenuItem } from "../../components/ui/MoreMenu";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { driveApi } from "../drive/api";
import {
  useMatchTrack,
  useUpdateTrack,
  useUploadCover,
  type Track,
} from "./api";
import { openAddToPlaylist, openEditLyrics, openEditTrack } from "./dialogs";
import { usePlayer } from "./player";

function icon(Icon: LucideIcon) {
  return createElement(Icon, { size: 14 });
}

/**
 * 一首歌的“更多”菜单项（B147）：歌曲列表、播放队列、播放列表里的歌共用。
 * extra 放这个列表特有的项，比如“从播放列表移除”。
 */
export function useTrackMenu() {
  const t = useT();
  const navigate = useNavigate();
  const update = useUpdateTrack();
  const matchTrack = useMatchTrack();
  const uploadCover = useUploadCover();

  const toggleFavorite = (track: Track) =>
    update.mutate(
      { id: track.id, patch: { favorite: !track.favorite } },
      {
        onSuccess: (next) => usePlayer.getState().patchTrack(next),
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );

  const showInDrive = async (track: Track) => {
    try {
      const item = await unwrap(
        driveApi.GET("/drive/items/{itemId}", {
          params: { path: { itemId: track.driveItemId } },
        }),
      );
      navigate(item.parentId ? `/drive?folder=${item.parentId}` : "/drive");
    } catch (e) {
      toast({ message: errorMessage(e), tone: "error" });
    }
  };

  const match = (track: Track, overwrite: boolean) => {
    toast(t("Matching…"));
    matchTrack.mutate(
      { id: track.id, overwrite },
      {
        onSuccess: (next) => {
          usePlayer.getState().patchTrack(next);
          toast(
            next.matchState === "matched"
              ? t("Matched")
              : next.matchState === "pending"
                ? t("Not sure. Choose from the candidates in To confirm.")
                : t("Nothing found"),
          );
        },
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );
  };

  const pickCover = (track: Track) => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = "image/jpeg,image/png,image/webp";
    input.onchange = () => {
      const file = input.files?.[0];
      if (!file) return;
      uploadCover.mutate(
        { id: track.id, file },
        {
          onSuccess: (next) => {
            usePlayer.getState().patchTrack(next);
            toast(t("Saved"));
          },
          onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
        },
      );
    };
    input.click();
  };

  const items = (
    track: Track,
    options: {
      /** 点“播放”时要做的事。列表里是从这首开始播整个列表。 */
      play: () => void;
      extra?: MoreMenuItem[];
    },
  ): MoreMenuItem[] => [
    { key: "play", label: t("Play"), icon: icon(Play), onSelect: options.play },
    {
      key: "next",
      label: t("Play next"),
      icon: icon(ListStart),
      onSelect: () => {
        usePlayer.getState().playNext([track]);
        toast(t("Will play next"));
      },
    },
    {
      key: "queue",
      label: t("Add to queue"),
      icon: icon(ListEnd),
      onSelect: () => {
        usePlayer.getState().enqueue([track]);
        toast(t("Added to the queue"));
      },
    },
    {
      key: "playlist",
      label: t("Add to playlist"),
      icon: icon(ListPlus),
      onSelect: () => openAddToPlaylist([track]),
    },
    {
      key: "favorite",
      label: track.favorite
        ? t("Remove from favorites")
        : t("Add to favorites"),
      icon: icon(track.favorite ? HeartOff : Heart),
      onSelect: () => toggleFavorite(track),
    },
    ...(options.extra ?? []),
    {
      key: "match",
      label: t("Match lyrics and cover"),
      icon: icon(Wand2),
      onSelect: () => match(track, false),
    },
    {
      key: "rematch",
      label: t("Match again and replace"),
      icon: icon(Wand2),
      onSelect: () => match(track, true),
    },
    {
      key: "lyrics",
      label: t("Edit lyrics"),
      icon: icon(FileText),
      onSelect: () => openEditLyrics(track),
    },
    {
      key: "cover",
      label: t("Upload cover"),
      icon: icon(ImagePlus),
      onSelect: () => pickCover(track),
    },
    {
      key: "edit",
      label: t("Edit song info"),
      icon: icon(Pencil),
      onSelect: () => openEditTrack(track),
    },
    {
      key: "drive",
      label: t("Show in Drive"),
      icon: icon(FolderOpen),
      onSelect: () => void showInDrive(track),
    },
  ];

  return { items, toggleFavorite };
}
