import { Box, Button, Center, HStack } from "@chakra-ui/react";
import { Info, Pencil } from "lucide-react";
import { useRef, useState } from "react";
import { UserAvatar } from "~/components/UserAvatar";
import { Dialog } from "~/components/ui/dialog";
import { toaster } from "~/components/ui/toaster";
import { Tooltip } from "~/components/ui/tooltip";
import { usePersonalSettingsI18n } from "~/features/barena/personalSettingsI18n";
import { showErrorToast } from "~/features/errors";
import { api } from "~/utils/api";
import { useSession } from "~/utils/auth-client";
import { processAvatarImage } from "./processAvatarImage";

/** The photo as the edit control — a pencil badge overlays it; click opens the dialog. */
function AvatarEditButton({
  name,
  image,
  label,
  isDisabled,
  onOpen,
}: {
  name: string | null;
  image: string | null;
  label: string;
  isDisabled: boolean;
  onOpen: () => void;
}) {
  return (
    <Box
      position="relative"
      role="button"
      tabIndex={isDisabled ? -1 : 0}
      aria-label={label}
      aria-disabled={isDisabled}
      cursor={isDisabled ? "default" : "pointer"}
      transition="opacity 0.15s"
      _hover={isDisabled ? undefined : { opacity: 0.85 }}
      onClick={isDisabled ? undefined : onOpen}
      onKeyDown={(e) => {
        if (!isDisabled && (e.key === "Enter" || e.key === " ")) {
          e.preventDefault();
          onOpen();
        }
      }}
    >
      <UserAvatar
        name={name}
        image={image}
        size="xl"
        borderWidth="1px"
        borderColor="border.muted"
      />
      <Box
        position="absolute"
        bottom="0"
        right="0"
        width="18px"
        height="18px"
        bg="orange.500"
        color="white"
        borderRadius="full"
        borderWidth="2px"
        borderColor="bg.surface"
        display="flex"
        alignItems="center"
        justifyContent="center"
      >
        <Pencil size={9} />
      </Box>
    </Box>
  );
}

/** LinkedIn-style "Profile photo" dialog — view the photo large, then change it. */
function AvatarPhotoDialog({
  isOpen,
  onOpenChange,
  name,
  image,
  hasImage,
  hasPreview,
  isBusy,
  isSaving,
  isRemoving,
  onChange,
  onRemove,
  onSave,
  onCancel,
}: {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  name: string | null;
  image: string | null;
  hasImage: boolean;
  hasPreview: boolean;
  isBusy: boolean;
  isSaving: boolean;
  isRemoving: boolean;
  onChange: () => void;
  onRemove: () => void;
  onSave: () => void;
  onCancel: () => void;
}) {
  const { t } = usePersonalSettingsI18n();
  return (
    <Dialog.Root
      open={isOpen}
      onOpenChange={(e) => onOpenChange(e.open)}
      size="sm"
    >
      <Dialog.Content bg="bg">
        <Dialog.Header>
          <Dialog.Title>{t("profilePhoto")}</Dialog.Title>
        </Dialog.Header>
        <Dialog.CloseTrigger aria-label={t("dismiss")} />
        <Dialog.Body>
          <Center py={2}>
            <UserAvatar
              name={name}
              image={image}
              width="180px"
              height="180px"
              borderWidth="1px"
              borderColor="border.muted"
            />
          </Center>
        </Dialog.Body>
        <Dialog.Footer>
          {hasPreview ? (
            <HStack gap={2}>
              <Button variant="ghost" onClick={onCancel} disabled={isBusy}>
                {t("cancel")}
              </Button>
              <Button
                colorPalette="orange"
                onClick={onSave}
                loading={isSaving}
                disabled={isBusy}
              >
                {t("savePhoto")}
              </Button>
            </HStack>
          ) : (
            <HStack gap={2}>
              {hasImage && (
                <Button
                  variant="ghost"
                  colorPalette="red"
                  onClick={onRemove}
                  loading={isRemoving}
                  disabled={isBusy}
                >
                  {t("remove")}
                </Button>
              )}
              <Button
                colorPalette="orange"
                onClick={onChange}
                disabled={isBusy}
              >
                {hasImage ? t("changePhoto") : t("uploadPhoto")}
              </Button>
            </HStack>
          )}
        </Dialog.Footer>
      </Dialog.Content>
    </Dialog.Root>
  );
}

/**
 * Profile-settings control for the user's avatar. Clicking the photo opens a
 * dialog where the current photo is shown large (view) and can be replaced
 * (change) — the format/size constraints live in the adjacent info tooltip.
 * The photo flows to every avatar surface via `User.image`.
 *
 * `organizationId` scopes the personal-workspace the photo is stored under.
 *
 * Spec: specs/settings/user-avatar.feature
 */
export function AvatarUploadControl({
  organizationId,
}: {
  organizationId: string;
}) {
  const { t } = usePersonalSettingsI18n();
  const session = useSession();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [preview, setPreview] = useState<string | null>(null);
  const [isProcessing, setIsProcessing] = useState(false);

  const currentImage = session.data?.user.image ?? null;
  const name = session.data?.user.name ?? session.data?.user.email ?? null;

  const setAvatar = api.user.setAvatar.useMutation({
    onSuccess: async () => {
      await session.update();
      setPreview(null);
      setIsOpen(false);
      toaster.create({ title: t("photoUpdated"), type: "success" });
    },
    onError: (error) =>
      showErrorToast({ error, fallbackTitle: t("updatePhotoError") }),
  });

  const removeAvatar = api.user.removeAvatar.useMutation({
    onSuccess: async () => {
      await session.update();
      setPreview(null);
      toaster.create({ title: t("photoRemoved"), type: "success" });
    },
    onError: (error) =>
      showErrorToast({ error, fallbackTitle: t("removePhotoError") }),
  });

  const isBusy = isProcessing || setAvatar.isPending || removeAvatar.isPending;

  const onFilePicked = async (file: File | undefined) => {
    // Allow re-selecting the same file later by clearing the input value.
    if (fileInputRef.current) fileInputRef.current.value = "";
    if (!file) return;
    setIsProcessing(true);
    try {
      setPreview(await processAvatarImage(file));
    } catch (err) {
      showErrorToast({
        error: err,
        fallbackTitle: t("readImageError"),
      });
    } finally {
      setIsProcessing(false);
    }
  };

  return (
    <>
      <HStack gap={2.5} align="center">
        <AvatarEditButton
          name={name}
          image={currentImage}
          label={currentImage ? t("editProfilePhoto") : t("addProfilePhoto")}
          isDisabled={isBusy}
          onOpen={() => setIsOpen(true)}
        />
        <Tooltip
          content={t("avatarFormatHint")}
          positioning={{ placement: "right" }}
        >
          <Box
            display="inline-flex"
            color="fg.muted"
            aria-label={t("avatarFormatHint")}
          >
            <Info size={16} />
          </Box>
        </Tooltip>
      </HStack>

      <input
        ref={fileInputRef}
        type="file"
        accept="image/png,image/jpeg,image/webp,image/gif"
        style={{ display: "none" }}
        onChange={(e) => void onFilePicked(e.target.files?.[0])}
      />

      <AvatarPhotoDialog
        isOpen={isOpen}
        onOpenChange={(open) => {
          if (isBusy) return;
          setIsOpen(open);
          if (!open) setPreview(null);
        }}
        name={name}
        image={preview ?? currentImage}
        hasImage={!!currentImage}
        hasPreview={!!preview}
        isBusy={isBusy}
        isSaving={setAvatar.isPending}
        isRemoving={removeAvatar.isPending}
        onChange={() => fileInputRef.current?.click()}
        onRemove={() => removeAvatar.mutate({})}
        onSave={() => {
          if (preview)
            setAvatar.mutate({ organizationId, imageDataUrl: preview });
        }}
        onCancel={() => setPreview(null)}
      />
    </>
  );
}
