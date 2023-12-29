import React from 'react';
import { MemoryRouter } from 'react-router';
import { render, screen, userEvent } from 'design/utils/testing';

import { ContextProvider } from 'teleport';

import { allAccessAcl, noAccess } from 'teleport/mocks/contexts';

import TeleportContext from 'teleport/teleportContext';

import { GitHubFlowProvider } from './useGitHubFlow';
import { GitHub } from './GitHub';

const tokenName = 'generated-test-token';
const authVersion = 'v15.0.0';

describe('gitHub', () => {
  afterEach(() => {
    jest.clearAllMocks();
  });

  type SetupProps = {
    hasAccess?: boolean;
  };

  const setup = ({ hasAccess = true }: SetupProps) => {
    const ctx = new TeleportContext();
    ctx.storeUser.setState({
      username: 'joe@example.com',
      acl: hasAccess ? allAccessAcl : { ...allAccessAcl, bots: noAccess },
      cluster: {
        authVersion: authVersion,
        clusterId: 'cluster-id',
        connectedText: 'connected-text',
        lastConnected: new Date('2023-01-01'),
        proxyVersion: 'v15.0.0',
        publicURL: 'publicurl',
        status: 'ok',
        url: 'url',
      },
    });

    jest.spyOn(ctx.resourceService, 'createRole').mockResolvedValue({
      id: 'role-id',
      kind: 'role',
      name: 'role-name',
      content: '',
    });
    jest.spyOn(ctx.joinTokenService, 'fetchJoinToken').mockResolvedValue({
      id: tokenName,
      expiry: new Date('2020-01-01'),
    });
    jest.spyOn(ctx.botService, 'createBot').mockResolvedValue();

    render(
      <MemoryRouter>
        <ContextProvider ctx={ctx}>
          <GitHubFlowProvider>
            <GitHub />
          </GitHubFlowProvider>
        </ContextProvider>
      </MemoryRouter>
    );
  };

  it('renders initial state with warning if user has no access', () => {
    setup({ hasAccess: false });
    expect(screen.getByText(/Insufficient permissions/)).toBeInTheDocument();
    expect(screen.getByTestId('button-next')).toBeDisabled();
  });

  it('renders initial state with no warnings if user the necessary access', () => {
    setup({});
    expect(
      screen.queryByText(/Insufficient permissions/)
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('button-next')).toBeEnabled();
  });

  it('allows the user to go through the whole flow', async () => {
    setup({});
    expect(
      screen.getByText(/Step 1: Scope the Permissions for Your Machine User/)
    ).toBeInTheDocument();
    // fill up the forms and go through the flow
    // step 1: Configure Bot Access
    const botNameInput = screen.getByPlaceholderText('ex. github-actions-cd');
    await userEvent.type(botNameInput, 'bot-name');
    await userEvent.click(screen.getByTestId('button-next'));
    // step 2: Connect GitHub
    expect(
      screen.getByText(/Step 2: Input Your GitHub Account Info/)
    ).toBeInTheDocument();
    const repositoryInput = screen.getByPlaceholderText(
      'ex. https://github.com/gravitational/teleport'
    );
    await userEvent.type(repositoryInput, 'https://github.com/owner/repo');
    await userEvent.click(screen.getByTestId('button-next'));
    // step 3: Add Bot to GitHub
    expect(
      screen.getByText(
        /Step 3: Connect Your Machine User in a GitHub Actions Workflow/
      )
    ).toBeInTheDocument();
    await userEvent.click(screen.getByTestId('button-next'));
    // Finish screen
    expect(
      screen.getByText(/Your Machine User is Added to Teleport/)
    ).toBeInTheDocument();
    expect(screen.getByText(/View Machine Users/)).toBeInTheDocument();
    expect(screen.getByText(/Add Another Integration/)).toBeInTheDocument();
  });
});
