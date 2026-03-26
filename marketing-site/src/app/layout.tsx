import { Analytics } from "@vercel/analytics/next";
import type { Metadata } from "next";
import { Archivo_Black, IBM_Plex_Mono, Space_Grotesk } from "next/font/google";
import "./globals.css";

const headline = Archivo_Black({
  subsets: ["latin"],
  weight: "400",
  variable: "--font-headline",
  display: "swap",
});

const body = Space_Grotesk({
  subsets: ["latin"],
  variable: "--font-body",
  display: "swap",
});

const mono = IBM_Plex_Mono({
  subsets: ["latin"],
  weight: ["400", "500", "600"],
  variable: "--font-mono",
  display: "swap",
});

export const metadata: Metadata = {
  title: "ROOM | Repetitively Obsessively Optimize Me",
  description:
    "ROOM is a tactile, operator-grade landing page for a CLI that runs cold-start repo improvement loops with Codex or Claude Code.",
  openGraph: {
    title: "ROOM | Repetitively Obsessively Optimize Me",
    description:
      "Recursive repo improvement with cold starts, artifact tape, forced pivots, and a live control-surface attitude.",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "ROOM | Repetitively Obsessively Optimize Me",
    description:
      "Recursive repo improvement with cold starts, artifact tape, forced pivots, and a live control-surface attitude.",
  },
  icons: {
    icon: [
      {
        url: "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 128 128'%3E%3Crect width='128' height='128' rx='24' fill='%23081214'/%3E%3Ccircle cx='39' cy='45' r='15' fill='%23d8ff45'/%3E%3Ccircle cx='88' cy='39' r='13' fill='%23ff6b3d'/%3E%3Cpath d='M24 91c9-22 26-33 50-33 18 0 29 6 44 21' stroke='%237df9ff' stroke-width='10' fill='none' stroke-linecap='round'/%3E%3C/svg%3E",
        type: "image/svg+xml",
      },
    ],
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${headline.variable} ${body.variable} ${mono.variable}`}
    >
      <body>
        {children}
        <Analytics />
      </body>
    </html>
  );
}
