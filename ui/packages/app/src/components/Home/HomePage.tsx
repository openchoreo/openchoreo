import { useState, useEffect } from 'react';

import { Content, Page, Header } from '@backstage/core-components';
import {
  HomePageRecentlyVisited,
  HomePageStarredEntities,
  HomePageToolkit,
  TemplateBackstageLogoIcon,
} from '@backstage/plugin-home';
import { HomePageSearchBar } from '@backstage/plugin-search';
import { SearchContextProvider } from '@backstage/plugin-search-react';
import { Grid, Typography, Card, CardContent, Box } from '@material-ui/core';
import {
  useApi,
  identityApiRef,
  errorApiRef,
} from '@backstage/core-plugin-api';
import { useStyles } from './styles';
import {
  DeveloperPortalWidget,
  HomePagePlatformDetailsCard,
  InfrastructureWidget,
} from '@openchoreo/backstage-plugin-platform-engineer-core';
import { MyProjectsWidget } from '@openchoreo/backstage-plugin';
import AddCircleOutlineIcon from '@material-ui/icons/AddCircleOutline';
import ViewListIcon from '@material-ui/icons/ViewList';
import AppsIcon from '@material-ui/icons/Apps';
import FeaturedPlayListOutlinedIcon from '@material-ui/icons/FeaturedPlayListOutlined';
/**
 * Custom HomePage that shows different content based on user groups
 */
export const HomePage = () => {
  const classes = useStyles();
  const identityApi = useApi(identityApiRef);
  const [userGroups, setUserGroups] = useState<string[]>([]);
  const [userName, setUserName] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const errorApi = useApi(errorApiRef);

  useEffect(() => {
    const loadUserInfo = async () => {
      try {
        const identity = await identityApi.getBackstageIdentity();
        const ownershipRefs = identity.ownershipEntityRefs || [];
        // Extract group names from refs like "group:default/admins"
        const groups = ownershipRefs
          .filter(ref => ref.startsWith('group:'))
          .map(ref => ref.split('/')[1]);
        setUserGroups(groups);
        setUserName(identity.userEntityRef.split('/')[1]);
      } catch (error) {
        errorApi.post(new Error('Failed to load user info:'));
      } finally {
        setLoading(false);
      }
    };

    loadUserInfo();
  }, [identityApi, errorApi]);

  // Determine user role based on groups
  const getUserRole = () => {
    if (userGroups.includes('platformengineer')) return 'platformengineer';
    if (userGroups.includes('developer')) return 'developer';
    return 'user';
  };

  // Get welcome message based on role
  const getWelcomeMessage = () => {
    const role = getUserRole();
    switch (role) {
      case 'platformengineer':
        return 'You have full administrative access to all features and resources.';
      case 'developer':
        return 'You can create and manage your own components and services.';
      default:
        return 'Welcome to the developer portal!';
    }
  };

  // Get quick actions based on role
  const getQuickActions = () => {
    if (userGroups.includes('admins') || userGroups.includes('managers')) {
      return [
        { title: 'Manage Users', link: '/catalog?filters[kind]=user' },
        {
          title: 'View All Components',
          link: '/catalog?filters[kind]=component',
        },
        { title: 'Create New Component', link: '/create' },
        { title: 'System Overview', link: '/catalog?filters[kind]=system' },
      ];
    }
    if (userGroups.includes('developers')) {
      return [
        {
          title: 'My Components',
          link: `/catalog?filters[kind]=component&filters[owners]=${userName}`,
        },
        { title: 'Create Component', link: '/create' },
        { title: 'Browse Catalog', link: '/catalog' },
        { title: 'Documentation', link: '/docs' },
      ];
    }
    // Viewers
    return [
      { title: 'Browse Catalog', link: '/catalog' },
      { title: 'Documentation', link: '/docs' },
      { title: 'API Docs', link: '/api-docs' },
      { title: 'Tech Radar', link: '/tech-radar' },
    ];
  };

  if (loading) {
    return (
      <Page themeId="home">
        <Header title="Loading..." />
        <Content>
          <Typography>Loading user information...</Typography>
        </Content>
      </Page>
    );
  }

  return (
    <SearchContextProvider>
      <Page themeId="home">
        <Header title={`Welcome, ${userName}!`} subtitle={getUserRole()} />
        <Content>
          <Grid container justifyContent="center" spacing={6}>
            {/* Search Bar */}
            <Grid container item xs={12} justifyContent="center">
              <HomePageSearchBar
                InputProps={{
                  classes: {
                    root: classes.searchBarInput,
                    notchedOutline: classes.searchBarOutline,
                  },
                }}
                placeholder="Search"
              />
            </Grid>

            {/* Welcome Card with Role Information */}
            <Grid container spacing={2}>
              <Grid item xs={12} md={6}>
                <Card className={classes.welcomeCard}>
                  <CardContent>
                    <Typography variant="h5" gutterBottom>
                      {getWelcomeMessage()}
                    </Typography>
                    <Typography
                      variant="body2"
                      color="textSecondary"
                      gutterBottom
                    >
                      Your groups:
                    </Typography>
                    <div>
                      {userGroups.map(group => (
                        <span key={group} className={classes.groupBadge}>
                          {group}
                        </span>
                      ))}
                    </div>
                  </CardContent>
                </Card>
              </Grid>

              {getUserRole() === 'platformengineer' && (
                <Grid item xs={12} md={8}>
                  {/* Platform Metrics Section */}
                  <Box className={classes.overviewSection}>
                    <Typography variant="h3">Platform Overview</Typography>
                    <Grid container className={classes.widgetContainer}>
                      <Grid item xs={12} md={5} sm={12}>
                        <InfrastructureWidget />
                      </Grid>
                      <Grid item xs={12} md={5} sm={12}>
                        <DeveloperPortalWidget />
                      </Grid>
                    </Grid>
                  </Box>

                  {/* Platform Details Section */}
                  <Box className={classes.platformDetailsSection}>
                    <HomePagePlatformDetailsCard />
                  </Box>
                </Grid>
              )}

              {getUserRole() === 'developer' && (
                <Grid item xs={12} md={8}>
                  {/* My Projects Widget Section */}
                  <Box className={classes.overviewSection}>
                    <Typography variant="h3">Overview</Typography>
                    <Grid container className={classes.widgetContainer}>
                      <Grid item xs={12} md={5} sm={12}>
                        <MyProjectsWidget />
                      </Grid>
                    </Grid>
                  </Box>
                </Grid>
              )}

              {/* Right Sidebar - Quick Access */}
              <Grid item xs={12} md={4}>
                <Grid container justifyContent="flex-end">
                  <Grid item md={10} xs={12} className={classes.sidebarSection}>
                    <Typography variant="h4" color="secondary">
                      Recent Activity
                    </Typography>

                    <Box className={classes.sidebarWidget}>
                      <HomePageStarredEntities />
                    </Box>

                    <Box className={classes.sidebarWidget}>
                      <HomePageRecentlyVisited />
                    </Box>
                  </Grid>
                </Grid>
              </Grid>

              {/* Quick Actions based on role */}
              {/* <Grid item xs={12} md={6}>
                <Card>
                  <CardContent>
                    <Typography variant="h6" gutterBottom>
                      Quick Actions
                    </Typography>
                    <Grid container spacing={2}>
                      {getQuickActions().map(action => (
                        <Grid item xs={6} key={action.title}>
                          <Card variant="outlined">
                            <CardContent>
                              <a
                                href={action.link}
                                style={{
                                  textDecoration: 'none',
                                  color: 'inherit',
                                }}
                              >
                                <Typography variant="body2">
                                  {action.title}
                                </Typography>
                              </a>
                            </CardContent>
                          </Card>
                        </Grid>
                      ))}
                    </Grid>
                  </CardContent>
                </Card>
              </Grid> */}
            </Grid>

            {/* Starred Entities - shown to all users */}
            {/* <Grid container item xs={12} md={6}>
              <HomePageStarredEntities />
            </Grid> */}

            {/* Toolkit - conditional based on role */}
            {(userGroups.includes('admins') ||
              userGroups.includes('managers') ||
              userGroups.includes('developers')) && (
              <Grid container item xs={12} md={6}>
                <HomePageToolkit
                  tools={[
                    {
                      url: '/create/templates/default/create-openchoreo-component',
                      label: 'Create Component',
                      icon: <AddCircleOutlineIcon />,
                    },
                    {
                      url: '/catalog-import',
                      label: 'Import Entity',
                      icon: <TemplateBackstageLogoIcon />,
                    },
                    {
                      url: '/catalog?filters[kind]=System&filters[user]=owned',
                      label: 'View My Projects',
                      icon: <ViewListIcon />,
                    },
                    {
                      url: '/catalog?filters[kind]=Component&filters[user]=owned',
                      label: 'View My Components',
                      icon: <AppsIcon />,
                    },
                    {
                      url: '/create',
                      label: 'Browse Templates',
                      icon: <FeaturedPlayListOutlinedIcon />,
                    },
                  ]}
                />
              </Grid>
            )}
          </Grid>
        </Content>
      </Page>
    </SearchContextProvider>
  );
};
