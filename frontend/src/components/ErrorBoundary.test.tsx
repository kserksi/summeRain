// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import i18n from "@/i18n";

const { reportClientCrash } = vi.hoisted(() => ({ reportClientCrash: vi.fn() }));

vi.mock("@/lib/crash-report", () => ({ reportClientCrash }));

import { ErrorBoundary } from "./ErrorBoundary";

function Bomb({ message }: { message: string }): ReactNode {
  throw new Error(message);
}

describe("ErrorBoundary", () => {
  beforeEach(() => {
    reportClientCrash.mockReset();
    // React logs caught render errors through console.error; these tests assert
    // the boundary's own behavior instead.
    vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the fallback and reports the crash", () => {
    render(
      <ErrorBoundary>
        <Bomb message="kaboom" />
      </ErrorBoundary>,
    );

    expect(screen.getByText(i18n.t("layout.errorTitle"))).toBeInTheDocument();
    expect(screen.getByText("kaboom")).toBeInTheDocument();
    expect(reportClientCrash).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "boundary", message: "kaboom" }),
    );
  });

  it("retries DOM-mutation crashes silently and reports after the budget", () => {
    render(
      <ErrorBoundary>
        <Bomb message="Failed to execute 'removeChild' on 'Node'" />
      </ErrorBoundary>,
    );

    expect(screen.getByText(i18n.t("layout.errorTitle"))).toBeInTheDocument();
    expect(reportClientCrash).toHaveBeenCalledTimes(1);
  });

  it("recovers when the retry button is pressed", async () => {
    let shouldThrow = true;
    function OneShot(): ReactNode {
      if (shouldThrow) throw new Error("first render fails");
      return <p>recovered</p>;
    }

    render(
      <ErrorBoundary>
        <OneShot />
      </ErrorBoundary>,
    );
    expect(screen.getByText(i18n.t("layout.errorTitle"))).toBeInTheDocument();

    shouldThrow = false;
    await userEvent.click(screen.getByRole("button", { name: i18n.t("common.retry") }));

    expect(screen.getByText("recovered")).toBeInTheDocument();
  });
});
