import { Heading, HStack, Text, VStack } from "@chakra-ui/react";
import { Languages, Monitor, Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import { HorizontalFormControl } from "~/components/HorizontalFormControl";
import SettingsLayout from "~/components/SettingsLayout";
import { SegmentedControl } from "~/components/ui/segmented-control";
import { type BarenaLocale, useBarenaI18n } from "~/features/barena/i18n";

type ThemeOption = "light" | "system" | "dark";

export default function PreferencesPage() {
  const { locale, setLocale, t } = useBarenaI18n();
  const { theme, setTheme } = useTheme();
  const selectedTheme: ThemeOption =
    theme === "light" || theme === "dark" ? theme : "system";

  return (
    <SettingsLayout>
      <VStack gap={5} width="full" maxWidth="900px" align="stretch">
        <VStack gap={1} align="start">
          <Heading as="h2" size="lg">
            {t("Appearance & Language")}
          </Heading>
          <Text fontSize="sm" color="fg.muted">
            {t("Choose how Catena looks and speaks on this browser.")}
          </Text>
        </VStack>

        <VStack gap={0} align="stretch">
          <HorizontalFormControl
            label={t("Language")}
            helper={t("Use Catena in your preferred interface language.")}
            inputWidth="360px"
          >
            <SegmentedControl
              width="full"
              value={locale}
              onValueChange={({ value }) =>
                value && setLocale(value as BarenaLocale)
              }
              items={[
                {
                  value: "zh-CN",
                  label: (
                    <HStack gap={2}>
                      <Languages size={14} />
                      <Text>{t("Chinese")}</Text>
                    </HStack>
                  ),
                },
                { value: "en", label: t("English") },
              ]}
            />
          </HorizontalFormControl>

          <HorizontalFormControl
            label={t("Theme")}
            helper={t("Follow your system or choose a fixed appearance.")}
            inputWidth="360px"
          >
            <SegmentedControl
              width="full"
              value={selectedTheme}
              onValueChange={({ value }) =>
                value && setTheme(value as ThemeOption)
              }
              items={[
                {
                  value: "light",
                  label: (
                    <HStack gap={2}>
                      <Sun size={14} />
                      <Text>{t("Light")}</Text>
                    </HStack>
                  ),
                },
                {
                  value: "system",
                  label: (
                    <HStack gap={2}>
                      <Monitor size={14} />
                      <Text>{t("System")}</Text>
                    </HStack>
                  ),
                },
                {
                  value: "dark",
                  label: (
                    <HStack gap={2}>
                      <Moon size={14} />
                      <Text>{t("Dark")}</Text>
                    </HStack>
                  ),
                },
              ]}
            />
          </HorizontalFormControl>
        </VStack>
      </VStack>
    </SettingsLayout>
  );
}
