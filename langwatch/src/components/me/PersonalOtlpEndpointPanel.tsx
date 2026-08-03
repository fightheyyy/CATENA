import { Box, Button, HStack, Spacer, Text, VStack } from "@chakra-ui/react";
import { Copy, Eye, EyeOff } from "lucide-react";
import { useState } from "react";
import { toaster } from "~/components/ui/toaster";
import { usePersonalSettingsI18n } from "~/features/barena/personalSettingsI18n";
import { usePublicEnv } from "~/hooks/usePublicEnv";

const SECRET_MASK = "•".repeat(36);

export function PersonalOtlpEndpointPanel({ apiKey }: { apiKey: string }) {
  const { t } = usePersonalSettingsI18n();
  const publicEnv = usePublicEnv();
  const baseHost = publicEnv.data?.BASE_HOST ?? "";
  const endpoint = baseHost ? `${baseHost}/api/otel` : "";
  const [showSecret, setShowSecret] = useState(false);

  const envVars = endpoint
    ? `export OTEL_EXPORTER_OTLP_ENDPOINT="${endpoint}"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer ${
        showSecret ? apiKey : SECRET_MASK
      }"`
    : "";

  const copyToClipboard = (value: string, label: string) => {
    void navigator.clipboard.writeText(value);
    toaster.create({
      title: t("copiedToClipboard", { label }),
      type: "success",
    });
  };

  return (
    <VStack align="stretch" gap={3}>
      <Row label={t("endpoint")}>
        <Text fontSize="sm" fontFamily="mono" wordBreak="break-all" flex={1}>
          {endpoint || "—"}
        </Text>
        <Button
          size="xs"
          variant="ghost"
          onClick={() => copyToClipboard(endpoint, t("endpoint"))}
          disabled={!endpoint}
        >
          <Copy size={12} /> {t("copy")}
        </Button>
      </Row>

      <Row label={t("apiKey")}>
        <Text fontSize="sm" fontFamily="mono" wordBreak="break-all" flex={1}>
          {apiKey ? (showSecret ? apiKey : SECRET_MASK) : "—"}
        </Text>
        <Button
          size="xs"
          variant="ghost"
          onClick={() => setShowSecret((v) => !v)}
          disabled={!apiKey}
        >
          {showSecret ? <EyeOff size={12} /> : <Eye size={12} />}
          {showSecret ? t("hide") : t("show")}
        </Button>
        <Button
          size="xs"
          variant="ghost"
          onClick={() => copyToClipboard(apiKey, t("apiKey"))}
          disabled={!apiKey}
        >
          <Copy size={12} /> {t("copy")}
        </Button>
      </Row>

      {envVars && (
        <Box
          borderWidth="1px"
          borderColor="border.muted"
          borderRadius="sm"
          padding={3}
          backgroundColor="bg.subtle"
        >
          <HStack alignItems="start" marginBottom={2}>
            <Text fontSize="xs" color="fg.muted" fontWeight="semibold">
              .env (bash)
            </Text>
            <Spacer />
            <Button
              size="xs"
              variant="ghost"
              onClick={() => {
                const text = endpoint
                  ? `export OTEL_EXPORTER_OTLP_ENDPOINT="${endpoint}"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer ${apiKey}"`
                  : "";
                copyToClipboard(text, t("envVars"));
              }}
              disabled={!apiKey}
            >
              <Copy size={12} /> {t("copy")}
            </Button>
          </HStack>
          <Box as="pre" fontSize="xs" fontFamily="mono" whiteSpace="pre-wrap">
            {envVars}
          </Box>
        </Box>
      )}

      <Text fontSize="xs" color="fg.muted">
        {t("customTelemetryDescription")}
      </Text>
    </VStack>
  );
}

function Row({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <HStack alignItems="center" gap={3}>
      <Text fontSize="sm" color="fg.muted" minWidth="80px">
        {label}
      </Text>
      {children}
    </HStack>
  );
}
